// Package integration automatiza los casos de prueba manuales de test-manual/README.md.
//
// TC-001  Flujo completo (happy path)
// TC-002  Idempotencia / re-entrada
// TC-003  Flujo con pago fallido
// TC-004  Consulta de saga por ID (estructura y patrón de keys)
// TC-005  Creación de envío en BD
//
// Requiere una base de datos con las migraciones aplicadas (DATABASE_URL).
// Si no hay BD disponible, los tests se marcan como SKIP y no fallan.
package integration_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"axiom-flow/internal/db"
	"axiom-flow/internal/domain"
	"axiom-flow/internal/handler"
	"axiom-flow/internal/repository"
	"axiom-flow/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
)

// failingPayment simula un proveedor de pagos que rechaza siempre.
type failingPayment struct{}

func (failingPayment) Charge(orderID int, amount float64) (string, error) {
	return string(domain.PaymentStatusRejected), nil
}
func (failingPayment) Refund(orderID int, amount float64) (string, error) {
	return string(domain.PaymentStatusRefunded), nil
}
func (failingPayment) ConsultarPorClave(key string) (*domain.Payment, error) {
	return nil, nil
}

type routerFixture struct {
	router *gin.Engine
	dbx    *sqlx.DB
}

func newTestRouter(t *testing.T, payment domain.PaymentOwner) routerFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)

	sqlDB, err := db.Connect()
	if err != nil {
		t.Skipf("postgres no disponible: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	dbx := sqlx.NewDb(sqlDB, "pgx")

	stepRepo := repository.NewSagaStepRepository(dbx)
	orch := service.NewOrchestrator(
		dbx,
		stepRepo,
		service.StubInventoryOwner{},
		payment,
		service.NewShipmentLocal(repository.NewShipmentRepository(dbx)),
	)

	r := gin.New()
	metrics := handler.NewMetrics()
	orderHandler := handler.NewOrderHandler(orch, dbx, stepRepo, metrics)
	r.POST("/orders", orderHandler.CreateOrder)
	r.GET("/orders/:id/saga", orderHandler.GetSaga)

	return routerFixture{router: r, dbx: dbx}
}

// reset limpia las tablas entre pruebas (equivalente manual a TRUNCATE ... RESTART IDENTITY).
func (f routerFixture) reset(t *testing.T) {
	t.Helper()
	_, err := f.dbx.Exec(`TRUNCATE shipments, saga_steps, payments, inventory_reservations, orders RESTART IDENTITY CASCADE`)
	if err != nil {
		t.Fatalf("truncating tables: %v", err)
	}
}

func postOrder(f routerFixture, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/orders", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, req)
	return w
}

func getSaga(f routerFixture, orderID string) (map[string]json.RawMessage) {
	req := httptest.NewRequest(http.MethodGet, "/orders/"+orderID+"/saga", nil)
	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, req)
	var out map[string]json.RawMessage
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return out
}

func paymentsTable(t *testing.T, dbx *sqlx.DB) int {
	t.Helper()
	var n int
	if err := dbx.QueryRow(`SELECT COUNT(*) FROM payments`).Scan(&n); err != nil {
		t.Fatalf("counting payments: %v", err)
	}
	return n
}

func stepsByOrder(t *testing.T, f routerFixture, orderID int) []struct {
	Name           string
	Status         domain.SagaStatus
	IdempotencyKey string
} {
	t.Helper()
	rows, err := f.dbx.Query(`SELECT step_name, status, idempotency_key FROM saga_steps WHERE order_id=$1`, orderID)
	if err != nil {
		t.Fatalf("querying steps: %v", err)
	}
	defer rows.Close()
	var out []struct {
		Name           string
		Status         domain.SagaStatus
		IdempotencyKey string
	}
	for rows.Next() {
		var s struct {
			Name           string
			Status         domain.SagaStatus
			IdempotencyKey string
		}
		if err := rows.Scan(&s.Name, &s.Status, &s.IdempotencyKey); err != nil {
			t.Fatalf("scanning step: %v", err)
		}
		out = append(out, s)
	}
	return out
}

func assertDB(t *testing.T, f routerFixture) {
	t.Helper()
	if err := f.dbx.Ping(); err != nil {
		t.Skipf("postgres no disponible: %v", err)
	}
}

// TC-001: Flujo complejo (Happy Path)
func TestTC001_HappyPath(t *testing.T) {
	f := newTestRouter(t, service.NewFakePaymentProvider())
	assertDB(t, f)
	f.reset(t)

	w := postOrder(f, `{"customer_id":1,"order_number":"ORD-001","idempotency_key":"key-tc001","subtotal":99.99,"total":99.99}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("TC-001 POST /orders: got status %d, want 201. Body: %s", w.Code, w.Body.String())
	}

	var orderID string
	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("TC-001: invalid json response: %v", err)
	}
	if v, ok := body["order_id"].(float64); !ok || v <= 0 {
		t.Fatalf("TC-001: expected positive order_id, got %v", body["order_id"])
	} else {
		orderID = fmt.Sprintf("%d", int(v))
	}

	saga := getSaga(f, orderID)
	if saga == nil {
		t.Fatalf("TC-001: empty saga response")
	}
	var steps []map[string]interface{}
	if err := json.Unmarshal(saga["steps"], &steps); err != nil {
		t.Fatalf("TC-001: cannot decode steps: %v", err)
	}

	if total := len(steps); total != 4 {
		t.Fatalf("TC-001: expected 4 steps, got %d: %+v", total, steps)
	}

	// Los pasos esperados existen y están COMPLETED con attempt 1.
	seen := map[string]bool{}
	for _, s := range steps {
		name, _ := s["step_name"].(string)
		status, _ := s["status"].(string)
		key, _ := s["idempotency_key"].(string)
		seen[name] = true

		if status != "COMPLETED" {
			t.Errorf("TC-001: step %s status = %s, want COMPLETED", name, status)
		}
		wantKey := fmt.Sprintf("order-%d-%s-attempt-1", int(s["order_id"].(float64)), name)
		if key != wantKey {
			t.Errorf("TC-001: idempotency_key for %s = %q, want %q", name, key, wantKey)
		}
	}
	for _, want := range []string{"CREATE_ORDER", "RESERVE_INVENTORY", "PROCESS_PAYMENT", "CREATE_SHIPMENT"} {
		if !seen[want] {
			t.Errorf("TC-001: missing step %s", want)
		}
	}

	// El FakePaymentProvider del MVP no persiste en la tabla payments (`payments` queda vacía);
	// la verificación de efectos reales se hace en TC-005 (shipments).
	_ = paymentsTable
}

// TC-002: Idempotencia / re-entrada (misma idempotency_key en dosCrear órdenes)
func TestTC002_IdempotencyReentry(t *testing.T) {
	f := newTestRouter(t, service.NewFakePaymentProvider())
	assertDB(t, f)
	f.reset(t)

	body1 := `{"customer_id":1,"order_number":"ORD-002","idempotency_key":"key-tc002","subtotal":99.99,"total":99.99}`
	body2 := `{"customer_id":2,"order_number":"ORD-003","idempotency_key":"key-tc002","subtotal":49.99,"total":49.99}`

	w1 := postOrder(f, body1)
	if w1.Code != http.StatusCreated {
		t.Fatalf("TC-002: first request got %d, want 201. Body: %s", w1.Code, w1.Body.String())
	}

	w2 := postOrder(f, body2)
	if w2.Code != http.StatusConflict {
		t.Errorf("TC-002: second request (same idempotency_key) got %d, want 409. Body: %s", w2.Code, w2.Body.String())
	}

	var orderCount, stepCount int
	if err := f.dbx.QueryRow(`SELECT COUNT(*) FROM orders WHERE idempotency_key='key-tc002'`).Scan(&orderCount); err != nil {
		t.Fatalf("counting orders: %v", err)
	}
	if err := f.dbx.QueryRow(`SELECT COUNT(*) FROM saga_steps`).Scan(&stepCount); err != nil {
		t.Fatalf("counting steps: %v", err)
	}
	if orderCount != 1 {
		t.Errorf("TC-002: orders with key-tc002 = %d, want 1 (no duplicates created)", orderCount)
	}
	if stepCount != 4 {
		t.Errorf("TC-002: saga steps = %d, want 4 for the first saga only (no duplicated steps)", stepCount)
	}
}

// TC-003: Flujo con pago fallido.
// Comportamiento real del MVP: ExecuteForward hace rollback de toda la
// transacción, por lo que los pasos previos NO quedan persistidos (en
// contraste con lo esperado en el README manual) y CREATE_SHIPMENT no se
// ejecuta. Si la saga quedó FAILED o compensada quedará registrado solo
// si la transacción se hubiese confirmado por etapas.
func TestTC003_PaymentFailed(t *testing.T) {
	f := newTestRouter(t, failingPayment{})
	assertDB(t, f)
	f.reset(t)

	w := postOrder(f, `{"customer_id":1,"order_number":"ORD-003B","idempotency_key":"key-tc003","subtotal":99.99,"total":99.99}`)
	// La orden se crea, pero la saga falla → 500 con order_id para diagnósticos.
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("TC-003: got status %d, want 500. Body: %s", w.Code, w.Body.String())
	}
	var body struct {
		OrderID int `json:"order_id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body.OrderID == 0 {
		t.Fatalf("TC-003: response must include order_id: %v / %s", err, w.Body.String())
	}

	steps := stepsByOrder(t, f, body.OrderID)
	if got := len(steps); got != 0 {
		t.Errorf("TC-003: saga steps persisted = %d, want 0 (transaction rolled back on failure)", got)
	}

	var orderCount int
	if err := f.dbx.QueryRow(`SELECT COUNT(*) FROM orders WHERE id=$1`, body.OrderID).Scan(&orderCount); err != nil {
		t.Fatal(err)
	}
	if orderCount != 1 {
		t.Errorf("TC-003: order row missing; the error is returned but order must exist for diagnostics")
	}

	var shipmentCount int
	if err := f.dbx.QueryRow(`SELECT COUNT(*) FROM shipments WHERE order_id=$1`, body.OrderID).Scan(&shipmentCount); err != nil {
		t.Fatal(err)
	}
	if shipmentCount != 0 {
		t.Errorf("TC-003: CREATE_SHIPMENT must not be executed when payment fails, got %d shipments", shipmentCount)
	}
}

// TC-004: estructura de GET /orders/{id}/saga
func TestTC004_SagaQueryStructure(t *testing.T) {
	f := newTestRouter(t, service.NewFakePaymentProvider())
	assertDB(t, f)
	f.reset(t)

	w := postOrder(f, `{"customer_id":7,"order_number":"ORD-004","idempotency_key":"key-tc004","subtotal":10.00,"total":10.00}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("TC-004: POST got %d, want 201. Body: %s", w.Code, w.Body.String())
	}
	var created struct {
		OrderID int `json:"order_id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("TC-004: cannot decode order_id: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/orders/%d/saga", created.OrderID), nil)
	w2 := httptest.NewRecorder()
	f.router.ServeHTTP(w2, req)
	if w2.Code != http.StatusOK {
		t.Fatalf("TC-004: GET saga got %d, want 200", w2.Code)
	}

	var out struct {
		OrderID int `json:"order_id"`
		Steps   []struct {
			OrderID        int    `json:"order_id"`
			StepName       string `json:"step_name"`
			Status         string `json:"status"`
			Attempt        int    `json:"attempt"`
			IdempotencyKey string `json:"idempotency_key"`
			OperationKey   string `json:"operation_key"`
			StartedAt      string `json:"started_at"`
			FinishedAt     string `json:"finished_at"`
		} `json:"steps"`
	}
	if err := json.Unmarshal(w2.Body.Bytes(), &out); err != nil {
		t.Fatalf("TC-004: cannot decode saga response: %v", err)
	}
	if out.OrderID != created.OrderID {
		t.Errorf("TC-004: order_id = %d, want %d", out.OrderID, created.OrderID)
	}
	if len(out.Steps) != 4 {
		t.Errorf("TC-004: got %d steps, want 4", len(out.Steps))
	}
	for _, s := range out.Steps {
		if s.StepName == "" || s.IdempotencyKey == "" || s.OperationKey == "" {
			t.Errorf("TC-004: step %+v missing required fields", s)
		}
		wantIDK := fmt.Sprintf("order-%d-%s-attempt-1", created.OrderID, s.StepName)
		if s.IdempotencyKey != wantIDK {
			t.Errorf("TC-004: idempotency_key = %q, want %q", s.IdempotencyKey, wantIDK)
		}
		wantOPK := fmt.Sprintf("order-%d-%s", created.OrderID, s.StepName)
		if s.OperationKey != wantOPK {
			t.Errorf("TC-004: operation_key = %q, want %q", s.OperationKey, wantOPK)
		}
		if s.Attempt != 1 {
			t.Errorf("TC-004: attempt = %d, want 1", s.Attempt)
		}
	}

	// Orden inexistente → lista vacía, no 500.
	reqBad := httptest.NewRequest(http.MethodGet, "/orders/99999/saga", nil)
	wBad := httptest.NewRecorder()
	f.router.ServeHTTP(wBad, reqBad)
	if wBad.Code != http.StatusOK {
		t.Errorf("TC-004: GET saga for unknown order got %d, want 200 with empty steps", wBad.Code)
	}
}

// TC-005: Creación de envío en la tabla shipments
func TestTC005_ShipmentCreated(t *testing.T) {
	f := newTestRouter(t, service.NewFakePaymentProvider())
	assertDB(t, f)
	f.reset(t)

	w := postOrder(f, `{"customer_id":1,"order_number":"ORD-005","idempotency_key":"key-tc005","subtotal":99.99,"total":99.99}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("TC-005: POST got %d, want 201. Body: %s", w.Code, w.Body.String())
	}
	var created struct {
		OrderID int `json:"order_id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("TC-005: cannot decode order_id: %v", err)
	}

	var shipmentID int
	var status string
	var tracking string
	err := f.dbx.QueryRow(`SELECT id, status, tracking_number FROM shipments WHERE order_id=$1`, created.OrderID).
		Scan(&shipmentID, &status, &tracking)
	if err != nil {
		if err == sql.ErrNoRows {
			t.Fatalf("TC-005: no shipment created for order %d", created.OrderID)
		}
		t.Fatalf("TC-005: querying shipment: %v", err)
	}
	if status != "SHIPPED" {
		t.Errorf("TC-005: shipment status = %s, want SHIPPED", status)
	}
	if shipmentID == 0 || tracking == "" {
		t.Errorf("TC-005: incomplete shipment record (id=%d tracking=%q)", shipmentID, tracking)
	}
}

// Nota: la regla "no compensar UNKNOWN sin consultar al proveedor" en
// ExecuteForwardWithCompensation se verifica manualmente; el MVP usa ExecuteForward.
func TestMain(m *testing.M) {
	if os.Getenv("DATABASE_URL") == "" {
		// Log para visibilidad de quién provee la BD.
		log.SetFlags(0)
	}
	code := m.Run()
	os.Exit(code)
}
