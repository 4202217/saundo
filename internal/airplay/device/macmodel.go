package device

import "strings"

// MacForm is the Mac hardware shape, used to pick a device glyph. It is
// derived from the model identifier the receiver advertises, not from the
// user-visible name.
type MacForm uint8

const (
	MacFormUnknown MacForm = iota
	MacFormLaptop
	MacFormMini
	MacFormStudio
	MacFormDesktop
)

func (f MacForm) String() string {
	switch f {
	case MacFormLaptop:
		return "laptop"
	case MacFormMini:
		return "mini"
	case MacFormStudio:
		return "studio"
	case MacFormDesktop:
		return "desktop"
	default:
		return "unknown"
	}
}

type macModel struct {
	form MacForm
	name string
}

// macModels maps Apple model identifiers to their hardware shape and
// marketing name. Source: kyle-seongwoo-jun/apple-device-identifiers
// (mac-device-identifiers.json), reviewed 2026-11-10; the M5 MacBook Pro and
// 2026 Mac Studio identifiers were added from Apple's model pages.
var macModels = map[string]macModel{
	"Mac13,1":        {form: MacFormStudio, name: "Mac Studio (2022)"},
	"Mac13,2":        {form: MacFormStudio, name: "Mac Studio (2022)"},
	"Mac14,10":       {form: MacFormLaptop, name: "MacBook Pro (16-inch, 2023)"},
	"Mac14,12":       {form: MacFormMini, name: "Mac mini (2023)"},
	"Mac14,13":       {form: MacFormStudio, name: "Mac Studio (2023)"},
	"Mac14,14":       {form: MacFormStudio, name: "Mac Studio (2023)"},
	"Mac14,15":       {form: MacFormLaptop, name: "MacBook Air (15-inch, M2, 2023)"},
	"Mac14,2":        {form: MacFormLaptop, name: "MacBook Air (M2, 2022)"},
	"Mac14,3":        {form: MacFormMini, name: "Mac mini (2023)"},
	"Mac14,5":        {form: MacFormLaptop, name: "MacBook Pro (14-inch, 2023)"},
	"Mac14,6":        {form: MacFormLaptop, name: "MacBook Pro (16-inch, 2023)"},
	"Mac14,7":        {form: MacFormLaptop, name: "MacBook Pro (13-inch, M2, 2022)"},
	"Mac14,8":        {form: MacFormDesktop, name: "Mac Pro (2023)"},
	"Mac14,9":        {form: MacFormLaptop, name: "MacBook Pro (14-inch, 2023)"},
	"Mac15,10":       {form: MacFormLaptop, name: "MacBook Pro (14-inch, Nov 2023)"},
	"Mac15,11":       {form: MacFormLaptop, name: "MacBook Pro (16-inch, Nov 2023)"},
	"Mac15,12":       {form: MacFormLaptop, name: "MacBook Air (13-inch, M3, 2024)"},
	"Mac15,13":       {form: MacFormLaptop, name: "MacBook Air (15-inch, M3, 2024)"},
	"Mac15,14":       {form: MacFormStudio, name: "Mac Studio (2025)"},
	"Mac15,3":        {form: MacFormLaptop, name: "MacBook Pro (14-inch, Nov 2023)"},
	"Mac15,4":        {form: MacFormDesktop, name: "iMac (24-inch, 2023, Two ports)"},
	"Mac15,5":        {form: MacFormDesktop, name: "iMac (24-inch, 2023, Four ports)"},
	"Mac15,6":        {form: MacFormLaptop, name: "MacBook Pro (14-inch, Nov 2023)"},
	"Mac15,7":        {form: MacFormLaptop, name: "MacBook Pro (16-inch, Nov 2023)"},
	"Mac15,8":        {form: MacFormLaptop, name: "MacBook Pro (14-inch, Nov 2023)"},
	"Mac15,9":        {form: MacFormLaptop, name: "MacBook Pro (16-inch, Nov 2023)"},
	"Mac16,1":        {form: MacFormLaptop, name: "MacBook Pro (14-inch, 2024)"},
	"Mac16,10":       {form: MacFormMini, name: "Mac mini (2024)"},
	"Mac16,11":       {form: MacFormMini, name: "Mac mini (2024)"},
	"Mac16,12":       {form: MacFormLaptop, name: "MacBook Air (13-inch, M4, 2025)"},
	"Mac16,13":       {form: MacFormLaptop, name: "MacBook Air (15-inch, M4, 2025)"},
	"Mac16,2":        {form: MacFormDesktop, name: "iMac (24-inch, 2024, Two ports)"},
	"Mac16,3":        {form: MacFormDesktop, name: "iMac (24-inch, 2024, Four ports)"},
	"Mac16,5":        {form: MacFormLaptop, name: "MacBook Pro (16-inch, 2024)"},
	"Mac16,6":        {form: MacFormLaptop, name: "MacBook Pro (14-inch, 2024)"},
	"Mac16,7":        {form: MacFormLaptop, name: "MacBook Pro (16-inch, 2024)"},
	"Mac16,8":        {form: MacFormLaptop, name: "MacBook Pro (14-inch, 2024)"},
	"Mac16,9":        {form: MacFormStudio, name: "Mac Studio (2025)"},
	"Mac17,14":       {form: MacFormStudio, name: "Mac Studio (2026)"},
	"Mac17,15":       {form: MacFormStudio, name: "Mac Studio (2026)"},
	"Mac17,2":        {form: MacFormLaptop, name: "MacBook Pro (14-inch, M5)"},
	"Mac17,6":        {form: MacFormLaptop, name: "MacBook Pro (14-inch, M5)"},
	"Mac17,7":        {form: MacFormLaptop, name: "MacBook Pro (14-inch, M5)"},
	"Mac17,8":        {form: MacFormLaptop, name: "MacBook Pro (16-inch, M5)"},
	"Mac17,9":        {form: MacFormLaptop, name: "MacBook Pro (16-inch, M5)"},
	"MacBook10,1":    {form: MacFormLaptop, name: "MacBook (Retina, 12-inch, 2017)"},
	"MacBook5,2":     {form: MacFormLaptop, name: "MacBook (13-inch, Early 2009)"},
	"MacBook6,1":     {form: MacFormLaptop, name: "MacBook (13-inch, Late 2009)"},
	"MacBook7,1":     {form: MacFormLaptop, name: "MacBook (13-inch, Mid 2010)"},
	"MacBook8,1":     {form: MacFormLaptop, name: "MacBook (Retina, 12-inch, Early 2015)"},
	"MacBook9,1":     {form: MacFormLaptop, name: "MacBook (Retina, 12-inch, Early 2016)"},
	"MacBookAir10,1": {form: MacFormLaptop, name: "MacBook Air (M1, 2020)"},
	"MacBookAir2,1":  {form: MacFormLaptop, name: "MacBook Air (Mid 2009)"},
	"MacBookAir3,1":  {form: MacFormLaptop, name: "MacBook Air (11-inch, Late 2010)"},
	"MacBookAir3,2":  {form: MacFormLaptop, name: "MacBook Air (13-inch, Late 2010)"},
	"MacBookAir4,1":  {form: MacFormLaptop, name: "MacBook Air (11-inch, Mid 2011)"},
	"MacBookAir4,2":  {form: MacFormLaptop, name: "MacBook Air (13-inch, Mid 2011)"},
	"MacBookAir5,1":  {form: MacFormLaptop, name: "MacBook Air (11-inch, Mid 2012)"},
	"MacBookAir5,2":  {form: MacFormLaptop, name: "MacBook Air (13-inch, Mid 2012)"},
	"MacBookAir6,1":  {form: MacFormLaptop, name: "MacBook Air (11-inch, Early 2014)"},
	"MacBookAir6,2":  {form: MacFormLaptop, name: "MacBook Air (13-inch, Early 2014)"},
	"MacBookAir7,1":  {form: MacFormLaptop, name: "MacBook Air (11-inch, Early 2015)"},
	"MacBookAir7,2":  {form: MacFormLaptop, name: "MacBook Air (13-inch, 2017)"},
	"MacBookAir8,1":  {form: MacFormLaptop, name: "MacBook Air (Retina, 13-inch, 2018)"},
	"MacBookAir8,2":  {form: MacFormLaptop, name: "MacBook Air (Retina, 13-inch, 2019)"},
	"MacBookAir9,1":  {form: MacFormLaptop, name: "MacBook Air (Retina, 13-inch, 2020)"},
	"MacBookPro10,1": {form: MacFormLaptop, name: "MacBook Pro (Retina, 15-inch, Early 2013)"},
	"MacBookPro10,2": {form: MacFormLaptop, name: "MacBook Pro (Retina, 13-inch, Early 2013)"},
	"MacBookPro11,1": {form: MacFormLaptop, name: "MacBook Pro (Retina, 13-inch, Late 2013)"},
	"MacBookPro11,2": {form: MacFormLaptop, name: "MacBook Pro (Retina, 15-inch, Late 2013)"},
	"MacBookPro11,3": {form: MacFormLaptop, name: "MacBook Pro (Retina, 15-inch, Late 2013)"},
	"MacBookPro11,4": {form: MacFormLaptop, name: "MacBook Pro (Retina, 15-inch, Mid 2015)"},
	"MacBookPro11,5": {form: MacFormLaptop, name: "MacBook Pro (Retina, 15-inch, Mid 2015)"},
	"MacBookPro12,1": {form: MacFormLaptop, name: "MacBook Pro (Retina, 13-inch, Early 2015)"},
	"MacBookPro13,1": {form: MacFormLaptop, name: "MacBook Pro (13-inch, 2016, Two Thunderbolt 3 ports)"},
	"MacBookPro13,2": {form: MacFormLaptop, name: "MacBook Pro (13-inch, 2016, Four Thunderbolt 3 ports)"},
	"MacBookPro13,3": {form: MacFormLaptop, name: "MacBook Pro (15-inch, 2016)"},
	"MacBookPro14,1": {form: MacFormLaptop, name: "MacBook Pro (13-inch, 2017, Two Thunderbolt 3 ports)"},
	"MacBookPro14,2": {form: MacFormLaptop, name: "MacBook Pro (13-inch, 2017, Four Thunderbolt 3 ports)"},
	"MacBookPro14,3": {form: MacFormLaptop, name: "MacBook Pro (15-inch, 2017)"},
	"MacBookPro15,1": {form: MacFormLaptop, name: "MacBook Pro (15-inch, 2018)"},
	"MacBookPro15,2": {form: MacFormLaptop, name: "MacBook Pro (13-inch, 2018, Four Thunderbolt 3 ports)"},
	"MacBookPro15,3": {form: MacFormLaptop, name: "MacBook Pro (15-inch, 2019)"},
	"MacBookPro15,4": {form: MacFormLaptop, name: "MacBook Pro (13-inch, 2019, Two Thunderbolt 3 ports)"},
	"MacBookPro16,1": {form: MacFormLaptop, name: "MacBook Pro (16-inch, 2019)"},
	"MacBookPro16,2": {form: MacFormLaptop, name: "MacBook Pro (13-inch, 2020, Four Thunderbolt 3 ports)"},
	"MacBookPro16,3": {form: MacFormLaptop, name: "MacBook Pro (13-inch, 2020, Two Thunderbolt 3 ports)"},
	"MacBookPro16,4": {form: MacFormLaptop, name: "MacBook Pro (16-inch, 2019)"},
	"MacBookPro17,1": {form: MacFormLaptop, name: "MacBook Pro (13-inch, M1, 2020)"},
	"MacBookPro18,1": {form: MacFormLaptop, name: "MacBook Pro (16-inch, 2021)"},
	"MacBookPro18,2": {form: MacFormLaptop, name: "MacBook Pro (16-inch, 2021)"},
	"MacBookPro18,3": {form: MacFormLaptop, name: "MacBook Pro (14-inch, 2021)"},
	"MacBookPro18,4": {form: MacFormLaptop, name: "MacBook Pro (14-inch, 2021)"},
	"MacBookPro4,1":  {form: MacFormLaptop, name: "MacBook Pro (15-inch, Early 2008)"},
	"MacBookPro5,1":  {form: MacFormLaptop, name: "MacBook Pro (15-inch, Late 2008)"},
	"MacBookPro5,2":  {form: MacFormLaptop, name: "MacBook Pro (17-inch, Early 2009)"},
	"MacBookPro5,3":  {form: MacFormLaptop, name: "MacBook Pro (15-inch, 2.53GHz, Mid 2009)"},
	"MacBookPro5,5":  {form: MacFormLaptop, name: "MacBook Pro (13-inch, Mid 2009)"},
	"MacBookPro6,1":  {form: MacFormLaptop, name: "MacBook Pro (17-inch, Mid 2010)"},
	"MacBookPro6,2":  {form: MacFormLaptop, name: "MacBook Pro (15-inch, Mid 2010)"},
	"MacBookPro7,1":  {form: MacFormLaptop, name: "MacBook Pro (13-inch, Mid 2010)"},
	"MacBookPro8,1":  {form: MacFormLaptop, name: "MacBook Pro (13-inch, Early 2011)"},
	"MacBookPro8,2":  {form: MacFormLaptop, name: "MacBook Pro (15-inch, Early 2011)"},
	"MacBookPro8,3":  {form: MacFormLaptop, name: "MacBook Pro (17-inch, Early 2011)"},
	"MacBookPro9,1":  {form: MacFormLaptop, name: "MacBook Pro (15-inch, Mid 2012)"},
	"MacBookPro9,2":  {form: MacFormLaptop, name: "MacBook Pro (13-inch, Mid 2012)"},
	"MacPro4,1":      {form: MacFormDesktop, name: "Mac Pro (Early 2009)"},
	"MacPro5,1":      {form: MacFormDesktop, name: "Mac Pro (Mid 2010)"},
	"MacPro6,1":      {form: MacFormDesktop, name: "Mac Pro (Late 2013)"},
	"MacPro7,1":      {form: MacFormDesktop, name: "Mac Pro (2019)"},
	"Macmini3,1":     {form: MacFormMini, name: "Mac mini (Early 2009)"},
	"Macmini4,1":     {form: MacFormMini, name: "Mac mini (Mid 2010)"},
	"Macmini5,1":     {form: MacFormMini, name: "Mac mini (Mid 2011)"},
	"Macmini5,2":     {form: MacFormMini, name: "Mac mini (Mid 2011)"},
	"Macmini6,1":     {form: MacFormMini, name: "Mac mini (Late 2012)"},
	"Macmini6,2":     {form: MacFormMini, name: "Mac mini (Late 2012)"},
	"Macmini7,1":     {form: MacFormMini, name: "Mac mini (Late 2014)"},
	"Macmini8,1":     {form: MacFormMini, name: "Mac mini (2018)"},
	"Macmini9,1":     {form: MacFormMini, name: "Mac mini (M1, 2020)"},
	"iMac10,1":       {form: MacFormDesktop, name: "iMac (21.5-inch, Late 2009)"},
	"iMac11,2":       {form: MacFormDesktop, name: "iMac (21.5-inch, Mid 2010)"},
	"iMac11,3":       {form: MacFormDesktop, name: "iMac (27-inch, Mid 2010)"},
	"iMac12,1":       {form: MacFormDesktop, name: "iMac (21.5-inch, Mid 2011)"},
	"iMac12,2":       {form: MacFormDesktop, name: "iMac (27-inch, Mid 2011)"},
	"iMac13,1":       {form: MacFormDesktop, name: "iMac (21.5-inch, Late 2012)"},
	"iMac13,2":       {form: MacFormDesktop, name: "iMac (27-inch, Late 2012)"},
	"iMac14,1":       {form: MacFormDesktop, name: "iMac (21.5-inch, Late 2013)"},
	"iMac14,2":       {form: MacFormDesktop, name: "iMac (27-inch, Late 2013)"},
	"iMac14,4":       {form: MacFormDesktop, name: "iMac (21.5-inch, Mid 2014)"},
	"iMac15,1":       {form: MacFormDesktop, name: "iMac (Retina 5K, 27-inch, Late 2014)"},
	"iMac16,1":       {form: MacFormDesktop, name: "iMac (21.5-inch, Late 2015)"},
	"iMac16,2":       {form: MacFormDesktop, name: "iMac (Retina 4K, 21.5-inch, Late 2015)"},
	"iMac17,1":       {form: MacFormDesktop, name: "iMac (Retina 5K, 27-inch, Late 2015)"},
	"iMac18,1":       {form: MacFormDesktop, name: "iMac (21.5-inch, 2017)"},
	"iMac18,2":       {form: MacFormDesktop, name: "iMac (Retina 4K, 21.5-inch, 2017)"},
	"iMac18,3":       {form: MacFormDesktop, name: "iMac (Retina 5K, 27-inch, 2017)"},
	"iMac19,1":       {form: MacFormDesktop, name: "iMac (Retina 5K, 27-inch, 2019)"},
	"iMac19,2":       {form: MacFormDesktop, name: "iMac (Retina 4K, 21.5-inch, 2019)"},
	"iMac20,1":       {form: MacFormDesktop, name: "iMac (Retina 5K, 27-inch, 2020)"},
	"iMac20,2":       {form: MacFormDesktop, name: "iMac (Retina 5K, 27-inch, 2020)"},
	"iMac21,1":       {form: MacFormDesktop, name: "iMac (24-inch, M1, 2021)"},
	"iMac21,2":       {form: MacFormDesktop, name: "iMac (24-inch, M1, 2021)"},
	"iMac9,1":        {form: MacFormDesktop, name: "iMac (20-inch, Early 2009)"},
	"iMacPro1,1":     {form: MacFormDesktop, name: "iMac Pro (2017)"},
}

// MacFormFor returns the hardware shape for a Mac model identifier. Known
// identifiers come from macModels; an identifier outside the table falls back
// to its family prefix, and any other Mac falls back to a desktop.
func MacFormFor(model string) MacForm {
	if m, ok := macModels[model]; ok {
		return m.form
	}
	switch {
	case strings.HasPrefix(model, "MacBook"):
		return MacFormLaptop
	case strings.HasPrefix(model, "Macmini"):
		return MacFormMini
	case strings.HasPrefix(model, "MacStudio"), strings.HasPrefix(model, "Macstudio"):
		return MacFormStudio
	case strings.HasPrefix(model, "iMac"), strings.HasPrefix(model, "MacPro"):
		return MacFormDesktop
	case strings.HasPrefix(model, "Mac"):
		return MacFormDesktop
	default:
		return MacFormUnknown
	}
}

// MacModelName returns the marketing name for a known Mac identifier, or ""
// when the identifier is not in the table.
func MacModelName(model string) string {
	if m, ok := macModels[model]; ok {
		return m.name
	}
	return ""
}
