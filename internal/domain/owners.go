package domain

// InventoryOwner is a stub owner for inventory reservation.
type InventoryOwner interface {
	Reserve(orderID int, productID int, quantity int) error
	Release(orderID int) error
}

// PaymentOwner is a stub owner for payment processing.
type PaymentOwner interface {
	Charge(orderID int, amount float64) (string, error)
	Refund(orderID int, amount float64) (string, error)
	ConsultarPorClave(key string) (*Payment, error)
}

// ShipmentOwner is a stub owner for shipment creation.
type ShipmentOwner interface {
	Create(orderID int, address string) (string, error)
}
