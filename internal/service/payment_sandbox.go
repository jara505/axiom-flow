package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"axiom-flow/internal/domain"
)

type paypalToken struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

type paypalOrderCreateRequest struct {
	Intent             string                   `json:"intent"`
	PurchaseUnits      []paypalPurchaseUnit     `json:"purchase_units"`
	ApplicationContext *paypalApplicationContext `json:"application_context,omitempty"`
}

type paypalPurchaseUnit struct {
	Amount      paypalAmount `json:"amount"`
	ReferenceID string       `json:"reference_id,omitempty"`
}

type paypalAmount struct {
	CurrencyCode string `json:"currency_code"`
	Value        string `json:"value"`
}

type paypalApplicationContext struct {
	ReturnURL string `json:"return_url,omitempty"`
	CancelURL string `json:"cancel_url,omitempty"`
}

type paypalOrderResponse struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

type paypalCaptureRequest struct {
	FinalCapture bool `json:"final_capture,omitempty"`
}

type paypalCaptureResponse struct {
	ID        string     `json:"id"`
	Status    string     `json:"status"`
	Amount    paypalAmount `json:"amount"`
}

type paypalRefundResponse struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}
type PayPalSandboxProvider struct {
	clientID     string
	clientSecret string
	baseURL      string
	httpClient   *http.Client
	tokenCache   string
	tokenExpiry  time.Time
}

func NewPayPalSandboxProvider() *PayPalSandboxProvider {
	return &PayPalSandboxProvider{
		clientID:     os.Getenv("PAYPAL_CLIENT_ID"),
		clientSecret: os.Getenv("PAYPAL_CLIENT_SECRET"),
		baseURL:      os.Getenv("PAYPAL_BASE_URL"),
		httpClient:   &http.Client{Timeout: 10 * time.Second},
	}
}

func (p *PayPalSandboxProvider) getAccessToken(ctx context.Context) (string, error) {
	if p.tokenCache != "" && time.Now().Before(p.tokenExpiry) {
		return p.tokenCache, nil
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/v1/oauth2/token", strings.NewReader("grant_type=client_credentials"))
	if err != nil {
		return "", fmt.Errorf("creando request de token: %w", err)
	}
	req.SetBasicAuth(p.clientID, p.clientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("obteniendo token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("error de autenticación PayPal: %s", string(body))
	}

	var token paypalToken
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		return "", fmt.Errorf("decodificando token: %w", err)
	}

	p.tokenCache = token.AccessToken
	p.tokenExpiry = time.Now().Add(time.Duration(token.ExpiresIn-60) * time.Second)

	return p.tokenCache, nil
}

func (p *PayPalSandboxProvider) doPayPalRequest(ctx context.Context, method, path string, body io.Reader, idempotencyKey string) (*http.Response, error) {
	token, err := p.getAccessToken(ctx)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, method, p.baseURL+path, body)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	if idempotencyKey != "" {
		req.Header.Set("PayPal-Request-Id", idempotencyKey)
	}

	return p.httpClient.Do(req)
}

func (p *PayPalSandboxProvider) Charge(orderID int, amount float64) (string, error) {
	ctx := context.Background()

	idempotencyKey := fmt.Sprintf("order-%d", orderID)

	orderReq := paypalOrderCreateRequest{
		Intent: "CAPTURE",
		PurchaseUnits: []paypalPurchaseUnit{
			{
				Amount: paypalAmount{
					CurrencyCode: "USD",
					Value:        fmt.Sprintf("%.2f", amount),
				},
				ReferenceID: fmt.Sprintf("order-%d", orderID),
			},
		},
	}

	body, err := json.Marshal(orderReq)
	if err != nil {
		return "", fmt.Errorf("marshal orden: %w", err)
	}

	resp, err := p.doPayPalRequest(ctx, "POST", "/v2/checkout/orders", strings.NewReader(string(body)), idempotencyKey)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("error creando orden PayPal: %s", string(respBody))
	}

	var orderResp paypalOrderResponse
	if err := json.NewDecoder(resp.Body).Decode(&orderResp); err != nil {
		return "", fmt.Errorf("decodificando orden: %w", err)
	}

	captureResp, err := p.doPayPalRequest(ctx, "POST", "/v2/checkout/orders/"+orderResp.ID+"/capture", strings.NewReader("{}"), idempotencyKey)
	if err != nil {
		return "", err
	}
	defer captureResp.Body.Close()

	if captureResp.StatusCode != http.StatusCreated && captureResp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(captureResp.Body)
		return "", fmt.Errorf("error capturando pago PayPal: %s", string(respBody))
	}

	var capture paypalCaptureResponse
	if err := json.NewDecoder(captureResp.Body).Decode(&capture); err != nil {
		return "", fmt.Errorf("decodificando captura: %w", err)
	}

	switch capture.Status {
	case "COMPLETED":
		return string(domain.PaymentStatusApproved), nil
	case "DECLINED":
		return string(domain.PaymentStatusRejected), nil
	default:
		return string(domain.PaymentStatusUnknown), nil
	}
}

func (p *PayPalSandboxProvider) Refund(orderID int, amount float64) (string, error) {
	ctx := context.Background()

	idempotencyKey := fmt.Sprintf("refund-order-%d", orderID)

	captureID := fmt.Sprintf("order-%d", orderID)

	body, err := json.Marshal(map[string]interface{}{
		"amount": paypalAmount{
			CurrencyCode: "USD",
			Value:        fmt.Sprintf("%.2f", amount),
		},
	})
	if err != nil {
		return "", fmt.Errorf("marshal reembolso: %w", err)
	}

	resp, err := p.doPayPalRequest(ctx, "POST", "/v2/payments/captures/"+captureID+"/refund", strings.NewReader(string(body)), idempotencyKey)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("error reembolsando PayPal: %s", string(respBody))
	}

	var refund paypalRefundResponse
	if err := json.NewDecoder(resp.Body).Decode(&refund); err != nil {
		return "", fmt.Errorf("decodificando reembolso: %w", err)
	}

	switch refund.Status {
	case "COMPLETED":
		return string(domain.PaymentStatusRefunded), nil
	default:
		return string(domain.PaymentStatusUnknown), nil
	}
}

var _ domain.PaymentOwner = (*PayPalSandboxProvider)(nil)

// ConsultarPorClave consulta el estado de un pago por su clave de idempotencia
func (p *PayPalSandboxProvider) ConsultarPorClave(key string) (*domain.Payment, error) {
	// En el sandbox, consultamos el estado del pago usando la clave
	// Por simplicidad en el sandbox, retornamos UNKNOWN para simular consulta
	payment := &domain.Payment{
		IdempotencyKey: key,
		Status:         "UNKNOWN",
	}
	return payment, nil
}
