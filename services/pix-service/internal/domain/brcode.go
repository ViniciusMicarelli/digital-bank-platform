package domain

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-common/money"
)

// BRCode is a Pix QR code payload ("Pix copia e cola"), the Central Bank's
// profile of EMV QRCPS-MPM: TLV fields (2-digit id, 2-digit length, value)
// ending with a CRC16-CCITT checksum.
type BRCode struct {
	Key          string
	Amount       *money.Money // nil: the payer chooses the amount
	MerchantName string
	MerchantCity string
	TxID         string // "***" when the code has no transaction id
	Description  string
	OneTime      bool // point of initiation "12": the code may be paid once
}

// EMV field ids used by Pix.
const (
	fieldFormat        = "00"
	fieldInitiation    = "01"
	fieldMerchantInfo  = "26"
	fieldMCC           = "52"
	fieldCurrency      = "53"
	fieldAmount        = "54"
	fieldCountry       = "58"
	fieldMerchantName  = "59"
	fieldMerchantCity  = "60"
	fieldAdditional    = "62"
	fieldCRC           = "63"
	pixGUI             = "br.gov.bcb.pix"
	subGUI, subKey     = "00", "01"
	subDescription     = "02"
	subTxID            = "05"
	maxMerchantName    = 25
	maxMerchantCity    = 15
	maxTxID            = 25
	maxQRDescription   = 72
	noTxID             = "***"
	currencyBRLNumeric = "986"
)

var txidRE = regexp.MustCompile(`^[A-Za-z0-9]{1,25}$`)

func tlv(id, value string) string { return fmt.Sprintf("%s%02d%s", id, len(value), value) }

// asciiUpper removes accents and keeps printable ASCII, as BR Code readers expect.
func asciiUpper(s string, max int) string {
	var b strings.Builder
	for _, r := range norm.NFD.String(s) {
		if unicode.Is(unicode.Mn, r) {
			continue // combining accent
		}
		if r >= 0x20 && r < 0x7f {
			b.WriteRune(unicode.ToUpper(r))
		}
	}
	out := strings.TrimSpace(b.String())
	if len(out) > max {
		out = strings.TrimSpace(out[:max])
	}
	return out
}

// Encode renders the payload, including the CRC.
func (c BRCode) Encode() (string, error) {
	if c.Key == "" {
		return "", fmt.Errorf("%w: QR code needs a Pix key", ErrInvalidArgument)
	}
	txid := c.TxID
	if txid == "" {
		txid = noTxID
	}
	if txid != noTxID && !txidRE.MatchString(txid) {
		return "", fmt.Errorf("%w: txid must be 1-25 letters or digits", ErrInvalidArgument)
	}
	account := tlv(subGUI, pixGUI) + tlv(subKey, c.Key)
	if d := asciiUpper(c.Description, maxQRDescription); d != "" {
		account += tlv(subDescription, d)
	}
	initiation := "11"
	if c.OneTime {
		initiation = "12"
	}
	var b strings.Builder
	b.WriteString(tlv(fieldFormat, "01"))
	b.WriteString(tlv(fieldInitiation, initiation))
	b.WriteString(tlv(fieldMerchantInfo, account))
	b.WriteString(tlv(fieldMCC, "0000"))
	b.WriteString(tlv(fieldCurrency, currencyBRLNumeric))
	if c.Amount != nil {
		if !c.Amount.IsPositive() || c.Amount.Currency() != money.BRL {
			return "", fmt.Errorf("%w: QR amount must be a positive BRL amount", ErrInvalidArgument)
		}
		b.WriteString(tlv(fieldAmount, c.Amount.Decimal()))
	}
	b.WriteString(tlv(fieldCountry, "BR"))
	b.WriteString(tlv(fieldMerchantName, asciiUpper(c.MerchantName, maxMerchantName)))
	b.WriteString(tlv(fieldMerchantCity, asciiUpper(c.MerchantCity, maxMerchantCity)))
	b.WriteString(tlv(fieldAdditional, tlv(subTxID, txid)))
	b.WriteString(fieldCRC + "04")
	payload := b.String()
	if len(payload) > 512 {
		return "", fmt.Errorf("%w: QR payload too long", ErrInvalidArgument)
	}
	return payload + fmt.Sprintf("%04X", CRC16(payload)), nil
}

// CRC16 is CRC-16/CCITT-FALSE (poly 0x1021, init 0xFFFF), as BR Code requires.
func CRC16(s string) uint16 {
	crc := uint16(0xFFFF)
	for i := 0; i < len(s); i++ {
		crc ^= uint16(s[i]) << 8
		for range 8 {
			if crc&0x8000 != 0 {
				crc = crc<<1 ^ 0x1021
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}

// parseTLV splits a TLV string into fields, rejecting malformed input.
func parseTLV(s string) (map[string]string, []string, error) {
	fields := map[string]string{}
	var order []string
	for i := 0; i < len(s); {
		if i+4 > len(s) {
			return nil, nil, fmt.Errorf("%w: truncated QR field", ErrInvalidArgument)
		}
		id := s[i : i+2]
		n, err := strconv.Atoi(s[i+2 : i+4])
		if err != nil || i+4+n > len(s) {
			return nil, nil, fmt.Errorf("%w: bad length in QR field %s", ErrInvalidArgument, id)
		}
		fields[id] = s[i+4 : i+4+n]
		order = append(order, id)
		i += 4 + n
	}
	return fields, order, nil
}

// DecodeBRCode validates the CRC and structure of a Pix payload.
func DecodeBRCode(payload string) (BRCode, error) {
	payload = strings.TrimSpace(payload)
	invalid := func(msg string) error { return fmt.Errorf("%w: %s", ErrInvalidArgument, msg) }
	if len(payload) < 8 || len(payload) > 512 || !strings.HasSuffix(payload[:len(payload)-4], fieldCRC+"04") {
		return BRCode{}, invalid("not a Pix QR code")
	}
	body, sum := payload[:len(payload)-4], payload[len(payload)-4:]
	if fmt.Sprintf("%04X", CRC16(body)) != strings.ToUpper(sum) {
		return BRCode{}, invalid("QR code checksum does not match (typo or tampering)")
	}
	f, order, err := parseTLV(payload)
	if err != nil {
		return BRCode{}, err
	}
	if len(order) == 0 || order[0] != fieldFormat || f[fieldFormat] != "01" {
		return BRCode{}, invalid("unsupported payload format")
	}
	if f[fieldCurrency] != currencyBRLNumeric || f[fieldCountry] != "BR" {
		return BRCode{}, invalid("only BRL Pix QR codes are supported")
	}
	acc, _, err := parseTLV(f[fieldMerchantInfo])
	if err != nil || !strings.EqualFold(acc[subGUI], pixGUI) || acc[subKey] == "" {
		return BRCode{}, invalid("QR code has no Pix key")
	}
	c := BRCode{
		Key:          acc[subKey],
		Description:  acc[subDescription],
		MerchantName: f[fieldMerchantName],
		MerchantCity: f[fieldMerchantCity],
		OneTime:      f[fieldInitiation] == "12",
		TxID:         noTxID,
	}
	if v, ok := f[fieldAmount]; ok {
		m, err := money.Parse(v, money.BRL)
		if err != nil || !m.IsPositive() {
			return BRCode{}, invalid("bad amount in QR code")
		}
		c.Amount = &m
	}
	if add, ok := f[fieldAdditional]; ok {
		sub, _, err := parseTLV(add)
		if err != nil {
			return BRCode{}, err
		}
		if t := sub[subTxID]; t != "" {
			c.TxID = t
		}
	}
	return c, nil
}

// HasTxID reports whether the code identifies a specific charge.
func (c BRCode) HasTxID() bool { return c.TxID != "" && c.TxID != noTxID }
