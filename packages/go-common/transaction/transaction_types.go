package transaction

// TransactionType represents the type of transaction.
type TransactionType string

const (
	// Payment is a payment transaction.
	Payment TransactionType = "PAY"
	
	// Deposit is a deposit transaction.
	Deposit TransactionType = "DEP"
	
	// Withdraw is a withdrawal transaction.
	Withdraw TransactionType = "WDR"
	
	// Transfer is a transfer transaction between accounts.
	Transfer TransactionType = "TRF"
	
	// PixPayment is a Pix payment transaction.
	PixPayment TransactionType = "PXM"
	
	// PixTransfer is a Pix transfer transaction.
	PixTransfer TransactionType = "PXF"
	
	// PixDeposit is a Pix deposit transaction.
	PixDeposit TransactionType = "PXD"
	
	// FX is a foreign exchange transaction.
	FX TransactionType = "FX"
	
	// Card is a card payment transaction.
	Card TransactionType = "CRD"
	
	// DirectDebit is a direct debit transaction.
	DirectDebit TransactionType = "DEB"
	
	// DirectCredit is a direct credit transaction.
	DirectCredit TransactionType = "CRE"
	
	// Loan is a loan transaction.
	Loan TransactionType = "LON"
	
	// Repayment is a loan repayment transaction.
	Repayment TransactionType = "RPM"
	
	// Interest is a loan interest transaction.
	Interest TransactionType = "INT"
	
	// Fee is a fee transaction.
	Fee TransactionType = "FEE"
	
	// Refund is a refund transaction.
	Refund TransactionType = "RFD"
)

// ValidateType validates the transaction type.
func ValidateType(t string) error {
	switch TransactionType(t) {
	case Payment, Deposit, Withdraw, Transfer, PixPayment,
		PixTransfer, PixDeposit, FX, Card, DirectDebit,
		DirectCredit, Loan, Repayment, Interest, Fee, Refund:
		return nil
	default:
		return fmt.Errorf("invalid transaction type: %s", t)
	}
}

// HasCurrency returns true if the transaction type has currency requirements.
func (t TransactionType) HasCurrency() bool {
	switch t {
	case Payment, Deposit, Withdraw, Transfer,
		FX, Card, DirectDebit, DirectCredit,
		Loan, Repayment, Interest:
		return true
	default:
		return false
	}
}

// IsPix returns true if the transaction is a Pix transaction.
func (t TransactionType) IsPix() bool {
	switch t {
	case PixPayment, PixTransfer, PixDeposit:
		return true
	default:
		return false
	}
}

// IsTransfer returns true if the transaction is a transfer.
func (t TransactionType) IsTransfer() bool {
	switch t {
	case Transfer, PixTransfer:
		return true
	default:
		return false
	}
}

// IsReverse returns true if the transaction is a reverse/credit type.
func (t TransactionType) IsReverse() bool {
	switch t {
	case Deposit, DirectCredit, Loan, Repayment, Interest, Refund:
		return true
	default:
		return false
	}
}

// IsAuth returns true if the transaction is an authorization.
func (t TransactionType) IsAuth() bool {
	switch t {
	case Payment, PixPayment, Card:
		return true
	default:
		return false
	}
}

// GetReverseType returns the reverse transaction type.
func (t TransactionType) GetReverseType() TransactionType {
	switch t {
	case Payment:
		return Deposit
	case Deposit:
		return Payment
	case Withdraw:
		return Deposit
	case Transfer:
		return Transfer
	case PixPayment:
		return PixDeposit
	case PixDeposit:
		return PixPayment
	case PixTransfer:
		return PixTransfer
	case FX:
		return FX
	case Card:
		return Card
	case DirectDebit:
		return DirectCredit
	case DirectCredit:
		return DirectDebit
	case Loan:
		return Loan
	case Repayment:
		return Repayment
	case Interest:
		return Interest
	case Fee:
		return Fee
	default:
		return ""
	}
}

// GetAuthType returns the authorization transaction type.
func (t TransactionType) GetAuthType() TransactionType {
	switch t {
	case Payment:
		return Payment
	case Deposit:
		return Deposit
	case Withdraw:
		return Withdraw
	case Transfer:
		return Transfer
	case PixPayment:
		return Payment
	case PixDeposit:
		return Deposit
	case PixTransfer:
		return Payment
	case Card:
		return Card
	case DirectDebit:
		return DirectDebit
	case DirectCredit:
		return DirectCredit
	default:
		return ""
	}
}

// GetCaptureType returns the capture transaction type.
func (t TransactionType) GetCaptureType() TransactionType {
	switch t {
	case Payment:
		return Payment
	case Deposit:
		return Deposit
	case Withdraw:
		return Withdraw
	case Transfer:
		return Transfer
	case PixPayment:
		return PixPayment
	case PixDeposit:
		return PixDeposit
	case PixTransfer:
		return PixTransfer
	case FX:
		return FX
	case Card:
		return Card
	default:
		return ""
	}
}