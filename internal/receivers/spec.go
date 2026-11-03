package receivers

import (
	"fmt"
	"net/netip"
	"strconv"
	"strings"
)

// Spec is what the add dialog submits. Password is transient: the engine
// moves it to the DPAPI store and never writes it to receivers.json.
type Spec struct {
	Name     string
	Type     Type
	Host     string
	Port     uint16
	Password string
}

// DefaultPort is the control port each family advertises by default.
func DefaultPort(t Type) uint16 {
	switch t {
	case TypeAirPlay2:
		return 7000
	default:
		// AirPort Express and AirPlay 1 both serve RAOP on 5000.
		return 5000
	}
}

// ValidType reports whether t is one of the add dialog's three families.
func ValidType(t Type) bool {
	switch t {
	case TypeAirPortExpress, TypeAirPlay1, TypeAirPlay2:
		return true
	}
	return false
}

// Field names the dialog field a validation error belongs to, so the UI can
// show a localized message beside the right control.
type Field uint8

const (
	FieldType Field = iota
	FieldHost
	FieldPort
)

// ValidationError is a Spec validation failure for one field.
type ValidationError struct {
	Field  Field
	Reason string
}

func (e *ValidationError) Error() string { return e.Reason }

func fieldError(field Field, format string, args ...any) error {
	return &ValidationError{Field: field, Reason: fmt.Sprintf(format, args...)}
}

// Validate checks the dialog's fields at the boundary the UI and the engine
// share. Name may be empty (the host stands in); the host must be a bare
// IP or hostname with no scheme, path, or port; a zero port takes the
// type's default.
func (s Spec) Validate() error {
	if !ValidType(s.Type) {
		return fieldError(FieldType, "unknown receiver type %q", s.Type)
	}
	host := strings.TrimSpace(s.Host)
	if host == "" {
		return fieldError(FieldHost, "host is required")
	}
	if strings.ContainsAny(host, "/\\:#?@") || strings.HasPrefix(host, "http") {
		return fieldError(FieldHost, "host %q is not a bare IP or hostname", host)
	}
	if _, err := netip.ParseAddr(host); err != nil {
		// Not an IP; accept it as a hostname if it looks like one.
		if len(host) > 253 {
			return fieldError(FieldHost, "host is too long")
		}
		// A trailing dot is the DNS root; the labels before it must stand.
		for _, label := range strings.Split(strings.TrimSuffix(host, "."), ".") {
			if label == "" || len(label) > 63 {
				return fieldError(FieldHost, "host %q is not a valid hostname", host)
			}
		}
	}
	return nil
}

// Resolved returns the spec with the type's default port applied.
func (s Spec) Resolved() Spec {
	if s.Port == 0 {
		s.Port = DefaultPort(s.Type)
	}
	return s
}

// Entry renders the spec as a stored manual entry with the given key.
func (s Spec) Entry(key string) Entry {
	return Entry{
		Key:  key,
		Kind: KindManual,
		Type: s.Type,
		Name: strings.TrimSpace(s.Name),
		Host: NormalizeHost(s.Host),
		Port: s.Port,
	}
}

// Patch changes one entry. A nil field leaves it alone. Password lives in
// the DPAPI store, not the registry; an empty string clears it.
type Patch struct {
	Name        *string
	Host        *string
	Port        *uint16
	Type        *Type
	Hidden      *bool
	AutoConnect *bool
	DelayMS     *int
	Password    *string
}

// Apply returns the entry with the patch's non-nil fields set. The engine
// calls it inside its registry save; the password field is handled
// separately against the DPAPI store.
func (p Patch) Apply(e Entry) Entry {
	if p.Name != nil {
		e.Name = strings.TrimSpace(*p.Name)
	}
	if p.Host != nil {
		e.Host = NormalizeHost(*p.Host)
	}
	if p.Port != nil {
		e.Port = *p.Port
	}
	if p.Type != nil && ValidType(*p.Type) {
		e.Type = *p.Type
	}
	if p.Hidden != nil {
		e.Hidden = *p.Hidden
	}
	if p.AutoConnect != nil {
		e.AutoConnect = *p.AutoConnect
	}
	if p.DelayMS != nil {
		e.DelayMS = ClampedDelay(*p.DelayMS)
	}
	return e
}

// PortText renders a manual address's port for the UI.
func PortText(p uint16) string { return strconv.Itoa(int(p)) }
