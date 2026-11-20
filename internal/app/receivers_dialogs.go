package app

import (
	"errors"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"saundo/cmd/saundo/locales"
	"saundo/internal/receivers"
)

// The receiver dialogs over the settings window: add/edit share one shell,
// the password and remove dialogs carry their own. All follow the unified
// dialog shape — icon head, body fields, a reserved error row, and the
// 34-DIP footer buttons.

// beginEditReceiver fills the draft from a manual row and opens the edit
// dialog.
func (a *App) beginEditReceiver(v receivers.View) {
	e, ok := a.receiverEntryFor(v.Key)
	if !ok {
		return
	}
	port := int(e.Port)
	if port == 0 {
		port = int(receivers.DefaultPort(e.Type))
	}
	d := receiverDraft{
		name:             v.Name,
		host:             e.Host,
		port:             strconv.Itoa(port),
		typeIdx:          receiverTypeIndex(e.Type),
		wantAuth:         v.PasswordSet,
		hasSavedPassword: v.PasswordSet,
	}
	d.typeLabelInit()
	d.syncAuthLabel()
	a.receiverDraft = d
	a.receiverDialog = receiverDialogState{kind: receiverDialogEdit, key: v.Key, entered: false}
	a.receiverDialogOpen = true
}

func receiverTypeIndex(t receivers.Type) int {
	for i, candidate := range receiverTypes {
		if candidate == t {
			return i
		}
	}
	return 0
}

// receiverEntryFor reads the registry entry behind a view through the
// snapshot; the UI never touches the registry directly.
func (a *App) receiverEntryFor(key string) (receivers.Entry, bool) {
	for _, v := range a.snap.Receivers {
		if v.Key == key {
			return v.Entry, true
		}
	}
	return receivers.Entry{}, false
}

// receiverViewFor finds a view by key.
func (a *App) receiverViewFor(key string) (receivers.View, bool) {
	for _, v := range a.snap.Receivers {
		if v.Key == key {
			return v, true
		}
	}
	return receivers.View{}, false
}

// receiverTypeLabelForLabel maps a select label back onto a Type.
func receiverTypeIndexForLabel(label string) int {
	for i, candidate := range receiverTypeLabels() {
		if candidate == label {
			return i
		}
	}
	return 0
}

// openAddReceiver resets the draft for a fresh manual receiver.
func (a *App) openAddReceiver() {
	a.receiverDraft = receiverDraft{port: "5000"}
	a.receiverDraft.typeLabelInit()
	a.receiverDialog = receiverDialogState{kind: receiverDialogAdd, entered: false}
	a.receiverDialogOpen = true
}

// openRemoveReceiver asks before deleting a manual row.
func (a *App) openRemoveReceiver(v receivers.View) {
	a.receiverDialog = receiverDialogState{kind: receiverDialogRemove, key: v.Key, entered: false}
	a.receiverDialogOpen = true
}

// closeReceiverDialog closes the open dialog and clears the draft.
func (a *App) closeReceiverDialog() {
	a.receiverDialogOpen = false
	a.receiverDialog = receiverDialogState{kind: receiverDialogNone, entered: true}
	a.receiverDraft = receiverDraft{}
}

func (a *App) receiverDialogs(c *ui.Context, p palette) {
	if a.receiverDialog.kind == receiverDialogNone {
		return
	}
	ui.DialogBase(c.Key("receiver-dialog"), &a.receiverDialogOpen, func(back, panel ui.Element) {
		back.Background(dialogScrim)
		panel.Width(430).MaxHeight(460).Padding(20).Radius(14).Background(p.card).
			Border(1, p.line).Shadow(0, 22, 54, 0, ui.RGBA(20, 22, 30, 0.26))
		dialogPose(c, panel, &a.receiverDialog.entered)
		switch a.receiverDialog.kind {
		case receiverDialogAdd, receiverDialogEdit:
			a.receiverAddEditDialog(c, p)
		case receiverDialogPassword:
			a.receiverPasswordDialog(c, p)
		case receiverDialogRemove:
			a.receiverRemoveDialog(c, p)
		case receiverDialogComboCreate:
			a.receiverComboCreateDialog(c, p)
		case receiverDialogComboUnlink:
			a.receiverComboUnlinkDialog(c, p)
		}
	})
}

// dialogHead is the icon-plus-title head the shells share.
func dialogHead(c *ui.Context, p palette, icon *ui.SVG, title, sub string, danger bool, trailing func()) {
	ui.Row(c).Gap(11).Children(func() {
		bg, fg := p.accentSoft, p.accent
		if danger {
			bg, fg = p.redSoft, p.red
		}
		ui.Box(c).Size(34, 34).Radius(10).Center().Shrink(0).Background(bg).Children(func() {
			ui.Icon(c, icon).Size(16, 16).TextColor(fg)
		})
		ui.Column(c).Grow(1).Gap(3).Children(func() {
			ui.Text(c, title).FontSize(18).FontWeight(650)
			ui.Text(c, sub).FontSize(12.5).TextColor(p.muted)
		})
		if trailing != nil {
			trailing()
		}
	})
}

// dialogErrorRow reserves the error line so the footer never jumps.
func dialogErrorRow(c *ui.Context, p palette, msg string) {
	ui.Text(c, msg).FontSize(11.5).TextColor(p.red).MinHeight(16).Margin(8, 0, 0, 0)
}

// dialogFooter is the action row with the divider above it.
func dialogFooter(c *ui.Context, p palette, left, right func()) {
	ui.Row(c).Margin(14, 0, 0, 0).Padding(14, 0, 0, 0).
		BorderWidth(1, 0, 0, 0).BorderColor(p.line2).Gap(8).AlignItems(ui.Center).Children(func() {
		if left != nil {
			left()
		}
		ui.Spacer(c)
		right()
	})
}

// fieldLabel is a small bold label above a field, with an optional hint.
func fieldLabel(c *ui.Context, p palette, label, hint string) {
	ui.Row(c).Gap(6).Margin(10, 0, 4, 0).AlignItems(ui.End).Children(func() {
		ui.Text(c, label).FontSize(11.5).FontWeight(600).LetterSpacing(0.2).TextColor(p.muted)
		if hint != "" {
			ui.Text(c, hint).FontSize(10.5).TextColor(p.faint).SingleLine().Ellipsis("…")
		}
	})
}

// dialogInput is the dialog's 32-DIP text field.
func dialogInput(c *ui.Context, p palette, value *string, placeholder, label string) ui.Element {
	in := ui.TextInput(c.Key(label), value).FillWidth().Height(32).Radius(8).PaddingX(10).
		FontSize(12.5).Label(label).Border(1, p.pill).Background(p.pill)
	if placeholder != "" {
		in.Placeholder(placeholder)
	}
	return in
}

func (a *App) receiverAddEditDialog(c *ui.Context, p palette) {
	editing := a.receiverDialog.kind == receiverDialogEdit
	d := &a.receiverDraft
	title := locales.ReceiverDialogTitleAdd()
	sub := locales.ReceiverDialogSubAdd()
	headIcon := iconPlus
	if editing {
		title = locales.ReceiverDialogTitleEdit()
		sub = locales.ReceiverDialogSubEdit()
		headIcon = iconPencil
	}

	ui.Column(c).Children(func() {
		dialogHead(c, p, headIcon, title, sub, false, nil)
		ui.Column(c).Margin(13, 0, 0, 0).Gap(0).Children(func() {
			ui.Row(c).Gap(10).Children(func() {
				ui.Column(c).Grow(1.3).Children(func() {
					fieldLabel(c, p, locales.ReceiverFieldName(), "")
					dialogInput(c, p, &d.name, locales.ReceiverFieldNamePlaceholder(), locales.ReceiverFieldName())
				})
				ui.Column(c).Grow(1).Children(func() {
					fieldLabel(c, p, locales.ReceiverFieldType(), "")
					if fieldSelect(c, p, &d.typeLabel, receiverTypeLabels(), locales.ReceiverFieldType()).Changed() {
						d.typeIdx = receiverTypeIndexForLabel(d.typeLabel)
						d.port = strconv.Itoa(int(receivers.DefaultPort(receiverTypes[d.typeIdx])))
					}
				})
			})
			ui.Column(c).Children(func() {
				fieldLabel(c, p, locales.ReceiverFieldHost(), "")
				dialogInput(c, p, &d.host, "192.168.1.12", locales.ReceiverFieldHost())
			})
			ui.Row(c).Gap(10).Children(func() {
				ui.Column(c).Width(132).Children(func() {
					fieldLabel(c, p, locales.ReceiverFieldPort(), locales.ReceiverPortHint())
					dialogInput(c, p, &d.port, "", locales.ReceiverFieldPort())
				})
				ui.Column(c).Grow(1).Children(func() {
					fieldLabel(c, p, locales.ReceiverFieldAuth(), locales.ReceiverFieldAuthHint())
					authOptions := []string{locales.ReceiverAuthNone(), locales.ReceiverAuthPassword()}
					d.syncAuthLabel()
					if fieldSelect(c, p, &d.authLabel, authOptions, locales.ReceiverFieldAuth()).Changed() {
						d.wantAuth = d.authLabel == locales.ReceiverAuthPassword()
						if !d.wantAuth {
							d.password = ""
						}
					}
				})
			})
			if d.wantAuth {
				ui.Column(c).Children(func() {
					fieldLabel(c, p, locales.ReceiverFieldPassword(), "")
					placeholder := locales.ReceiverPasswordPlaceholderNew()
					if editing && d.hasSavedPassword {
						placeholder = locales.ReceiverPasswordPlaceholderKeep()
					}
					dialogInput(c, p, &d.password, placeholder, locales.ReceiverFieldPassword()).Password()
				})
			}
		})
		dialogErrorRow(c, p, d.errorMsg)
		var left func()
		if editing {
			left = func() {
				quietDangerButton(c, p, locales.ReceiverRemove(), func() {
					if v, ok := a.receiverViewFor(a.receiverDialog.key); ok {
						a.openRemoveReceiver(v)
					}
				})
			}
		}
		dialogFooter(c, p, left,
			func() {
				quietButton(c, p, locales.ActionCancel(), func() { a.closeReceiverDialog() })
				if editing {
					primaryButton(c, p, locales.ReceiverDialogSave(), func() { a.submitReceiverDialog() })
				} else {
					primaryButton(c, p, locales.ReceiverDialogAdd(), func() { a.submitReceiverDialog() })
				}
			},
		)
	})
}

// submitReceiverDialog validates and sends the add or update command. The
// same validation the engine runs decides whether the dialog closes.
func (a *App) submitReceiverDialog() {
	d := &a.receiverDraft
	editing := a.receiverDialog.kind == receiverDialogEdit
	name := strings.TrimSpace(d.name)
	host := strings.TrimSpace(d.host)
	if name == "" {
		d.errorMsg = locales.ReceiverErrorName()
		return
	}
	if host == "" {
		d.errorMsg = locales.ReceiverErrorHost()
		return
	}
	port, err := strconv.Atoi(strings.TrimSpace(d.port))
	if err != nil || port < 1 || port > 65535 {
		d.errorMsg = locales.ReceiverErrorPort()
		return
	}
	spec := receivers.Spec{
		Name: name,
		Type: receiverTypes[d.typeIdx],
		Host: host,
		Port: uint16(port),
	}
	if err := spec.Validate(); err != nil {
		var ve *receivers.ValidationError
		switch {
		case errors.As(err, &ve) && ve.Field == receivers.FieldHost:
			d.errorMsg = locales.ReceiverErrorHost()
		case errors.As(err, &ve) && ve.Field == receivers.FieldPort:
			d.errorMsg = locales.ReceiverErrorPort()
		default:
			d.errorMsg = err.Error()
		}
		return
	}
	if d.wantAuth {
		password := d.password
		if editing && password == "" && d.hasSavedPassword {
			// Keep the saved one.
		} else if password == "" {
			d.errorMsg = locales.ReceiverErrorPassword()
			return
		} else {
			spec.Password = password
		}
	}
	if editing {
		name := spec.Name
		host := spec.Host
		port16 := spec.Port
		typ := spec.Type
		patch := receivers.Patch{Name: &name, Host: &host, Port: &port16, Type: &typ}
		if spec.Password != "" {
			patch.Password = &spec.Password
		} else if !d.wantAuth && d.hasSavedPassword {
			// The mockup's auth select set to 无 clears the saved password.
			empty := ""
			patch.Password = &empty
		}
		a.eng.UpdateReceiver(a.receiverDialog.key, patch)
	} else {
		a.eng.AddReceiver(spec)
	}
	a.closeReceiverDialog()
}

func (a *App) receiverPasswordDialog(c *ui.Context, p palette) {
	key := a.receiverDialog.key
	v, ok := a.receiverViewFor(key)
	if !ok {
		a.closeReceiverDialog()
		return
	}
	title := locales.ReceiverPasswordDialogTitleSet()
	if v.PasswordSet {
		title = locales.ReceiverPasswordDialogTitleChange()
	}
	d := &a.receiverDraft
	ui.Column(c).Children(func() {
		dialogHead(c, p, iconKey, title, locales.ReceiverPasswordDialogSub(v.Name), false, nil)
		ui.Column(c).Margin(13, 0, 0, 0).Children(func() {
			fieldLabel(c, p, locales.ReceiverPasswordNewLabel(), "")
			dialogInput(c, p, &d.password, locales.ReceiverPasswordPlaceholderNew(), locales.ReceiverPasswordNewLabel()).Password()
			ui.Text(c, locales.ReceiverPasswordDialogHint()).FontSize(11.5).
				TextColor(p.muted).Margin(8, 0, 0, 0)
		})
		dialogErrorRow(c, p, d.errorMsg)
		dialogFooter(c, p,
			func() {
				if v.PasswordSet {
					quietDangerButton(c, p, locales.ReceiverPasswordClear(), func() {
						a.eng.SetReceiverPassword(key, "")
						a.closeReceiverDialog()
					})
				} else {
					ui.Spacer(c)
				}
			},
			func() {
				quietButton(c, p, locales.ActionCancel(), func() { a.closeReceiverDialog() })
				primaryButton(c, p, locales.ReceiverDialogSave(), func() {
					if strings.TrimSpace(d.password) == "" {
						d.errorMsg = locales.ReceiverErrorPasswordRequired()
						return
					}
					a.eng.SetReceiverPassword(key, d.password)
					a.closeReceiverDialog()
				})
			},
		)
	})
}

func (a *App) receiverRemoveDialog(c *ui.Context, p palette) {
	key := a.receiverDialog.key
	v, ok := a.receiverViewFor(key)
	if !ok {
		a.closeReceiverDialog()
		return
	}
	model := receiverModelLabel(v)
	ui.Column(c).Children(func() {
		dialogHead(c, p, iconSlash, locales.ReceiverRemoveConfirmTitle(),
			locales.ReceiverRemoveConfirmSub(v.Name, model), true, nil)
		ui.Column(c).Margin(13, 0, 0, 0).Gap(5).Children(func() {
			ui.Text(c, locales.ReceiverRemoveHint1()).FontSize(11.5).TextColor(p.muted)
			ui.Text(c, locales.ReceiverRemoveHint2()).FontSize(11.5).TextColor(p.muted)
		})
		dialogErrorRow(c, p, "")
		dialogFooter(c, p,
			func() { ui.Spacer(c) },
			func() {
				quietButton(c, p, locales.ActionCancel(), func() { a.closeReceiverDialog() })
				primaryDangerButton(c, p, locales.ReceiverRemoveConfirm(), func() {
					delete(a.receiverOpen, key)
					delete(a.mutedVolume, key)
					a.eng.RemoveReceiver(key)
					a.closeReceiverDialog()
				})
			},
		)
	})
}

// quietDangerButton is the dialog's red text action.
func quietDangerButton(c *ui.Context, p palette, label string, fn func()) {
	b := ui.ButtonBase(c).Height(34).Radius(17).Padding(0, 12).Gap(5).Center().Cursor(ui.CursorPointer).Label(label)
	hoverFill(b, p.redSoft, 17, 0)
	b.OnClick(fn)
	b.Children(func() {
		ui.Icon(c, iconSlash).Size(13, 13).TextColor(p.red)
		ui.Text(c, label).FontSize(13).TextColor(p.red).SingleLine()
	})
}

// primaryDangerButton is the dialog's red commit button.
func primaryDangerButton(c *ui.Context, p palette, label string, fn func()) {
	b := ui.ButtonBase(c).Height(34).Radius(17).Padding(0, 16).Center().Cursor(ui.CursorPointer).Label(label)
	b.Background(p.red)
	hoverFill(b, p.red.Mix(p.bg, 0.12), 17, 0)
	b.OnClick(fn)
	b.Children(func() {
		ui.Text(c, label).FontSize(13).FontWeight(500).TextColor(p.bg).SingleLine()
	})
}

// comboCandidates lists the receivers a combo may link: visible, not the
// local host, not a native pair or an existing combo, and AirPlay 2 — the
// per-packet channel mix runs on the ALAC stream only.
func (a *App) comboCandidates() []receivers.View {
	var out []receivers.View
	for _, v := range a.snap.Receivers {
		if v.Hidden || v.Local || v.Entry.IsCombo() || v.Entry.IsGroup() || v.Device.IsStereoGroup() {
			continue
		}
		if !v.Device.AirPlay2 {
			continue
		}
		out = append(out, v)
	}
	return out
}

// comboCandidatesExcept is one side's picker rows: every eligible receiver
// except the one the other side already chose. A receiver cannot play both
// sides, so offering it twice is a dead option.
func comboCandidatesExcept(candidates []receivers.View, excludeKey string) []receivers.View {
	out := make([]receivers.View, 0, len(candidates))
	for _, v := range candidates {
		if v.Key == excludeKey {
			continue
		}
		out = append(out, v)
	}
	return out
}

// comboSideView is the receiver behind one side's key.
func comboSideView(candidates []receivers.View, key string) (receivers.View, bool) {
	for _, v := range candidates {
		if v.Key == key {
			return v, true
		}
	}
	return receivers.View{}, false
}

// comboSideLabel is the label one slot shows for its key.
func comboSideLabel(candidates []receivers.View, key string) string {
	if v, ok := comboSideView(candidates, key); ok {
		return v.Name
	}
	return ""
}

// comboPickSelect is one side's receiver picker: a field-shaped trigger that
// shows the choice's glyph and name, and a popup whose rows carry the same
// glyph, so both pickers name receivers rather than bare strings. The empty
// row clears the side. The rows leave out the receiver the other side holds.
func (a *App) comboPickSelect(c *ui.Context, p palette, value *string,
	candidates []receivers.View, excludeKey, label string, changed func()) {
	sel := ui.SelectBase(c, value)
	trigger := sel.Trigger.FillWidth().Height(32).Radius(8).PaddingX(10).
		Border(1, p.pill).Background(p.pill).Cursor(ui.CursorPointer).Label(label)
	trigger.Children(func() {
		ui.Row(c).Grow(1).Gap(8).AlignItems(ui.Center).Children(func() {
			if v, ok := comboSideView(candidates, *value); ok {
				ui.Icon(c, deviceIcon(v.Model, false)).Size(14, 14).TextColor(p.fg2).Shrink(0)
				ui.Text(c, v.Name).FontSize(12.5).SingleLine().Ellipsis("…").Shrink(1)
			} else {
				ui.Text(c, locales.ComboPickEmpty()).FontSize(12.5).TextColor(p.muted).Shrink(1)
			}
			ui.Spacer(c)
			ui.Icon(c, iconChevron).Size(12, 12).TextColor(p.muted).Shrink(0)
		})
	})
	sel.Popup(func(panel ui.Element) {
		panel.Padding(4).Radius(9).Background(p.card).Border(1, p.line)
		panel.Children(func() {
			row := func(v *receivers.View, name string) {
				item := sel.Item("")
				if v != nil {
					item = sel.Item(v.Key)
				}
				item.Height(28).Radius(7).PaddingX(8).Gap(8).AlignItems(ui.Center).
					Cursor(ui.CursorPointer).Label(name)
				if item.Highlighted() {
					item.Background(p.sel)
				}
				item.Children(func() {
					if v != nil {
						ui.Icon(c, deviceIcon(v.Model, false)).Size(14, 14).TextColor(p.fg2)
						ui.Text(c, name).FontSize(12.5).SingleLine().Ellipsis("…")
						return
					}
					ui.Text(c, name).FontSize(12.5).TextColor(p.muted).SingleLine()
				})
			}
			row(nil, locales.ComboPickEmpty())
			rows := comboCandidatesExcept(candidates, excludeKey)
			for i := range rows {
				row(&rows[i], rows[i].Name)
			}
		})
	})
	if trigger.Changed() {
		changed()
	}
}

// receiverComboCreateDialog builds the combo as a plain form, the same
// label-plus-field shape the add dialog uses: one select per side, the name
// with the derived suggestion as its placeholder, and 组合 enabling when both
// sides hold a receiver. Each select's first option is the empty state's
// call to action, so an untouched dialog never shows a blank control.
func (a *App) receiverComboCreateDialog(c *ui.Context, p palette) {
	d := &a.receiverDialog
	candidates := a.comboCandidates()
	// A slot whose receiver left the picker clears, so the form cannot hold a
	// stale key the selects no longer show.
	if d.comboLeft != "" && comboSideLabel(candidates, d.comboLeft) == "" {
		d.comboLeft = ""
	}
	if d.comboRight != "" && comboSideLabel(candidates, d.comboRight) == "" {
		d.comboRight = ""
	}
	leftLabel := comboSideLabel(candidates, d.comboLeft)
	rightLabel := comboSideLabel(candidates, d.comboRight)
	ready := d.comboLeft != "" && d.comboRight != ""

	ui.Column(c).Children(func() {
		dialogHead(c, p, iconLink, locales.ReceiverPairAddTitle(), locales.ComboDialogSub(), false, nil)
		ui.Column(c).Margin(13, 0, 0, 0).Children(func() {
			fieldLabel(c, p, locales.ComboFieldLeft(), "")
			a.comboPickSelect(c, p, &d.comboLeft, candidates, d.comboRight,
				locales.ComboFieldLeft(), func() { d.comboError = "" })
			fieldLabel(c, p, locales.ComboFieldRight(), "")
			a.comboPickSelect(c, p, &d.comboRight, candidates, d.comboLeft,
				locales.ComboFieldRight(), func() { d.comboError = "" })
			// One muted line under the fields explains why a receiver may be
			// missing from the pickers.
			ui.Text(c, locales.ComboPickHint()).FontSize(11).TextColor(p.muted).Margin(6, 0, 0, 0)
			fieldLabel(c, p, locales.ReceiverFieldName(), "")
			suggest := ""
			if ready {
				suggest = leftLabel + " + " + rightLabel
			}
			dialogInput(c, p, &d.comboName, suggest, locales.ReceiverFieldName())
			dialogErrorRow(c, p, d.comboError)
		})
		dialogFooter(c, p,
			func() { ui.Spacer(c) },
			func() {
				quietButton(c, p, locales.ActionCancel(), func() { a.closeReceiverDialog() })
				if ready {
					primaryButton(c, p, locales.ComboCreate(), func() { a.submitComboCreate() })
				} else {
					// The commit button shows its 40% disabled pose until both
					// sides hold a receiver.
					b := ui.ButtonBase(c).Height(34).Radius(17).Padding(0, 16).Center().
						Label(locales.ComboCreate())
					b.Background(p.fg)
					b.Disabled(true)
					b.Opacity(0.4)
					b.Children(func() {
						ui.Text(c, locales.ComboCreate()).FontSize(13).FontWeight(500).TextColor(p.bg).SingleLine()
					})
				}
			},
		)
	})
}

// submitComboCreate validates the two slots against the picker's current
// candidates and sends the link command. An invalid slot keeps the dialog
// open with the error line, because the engine's refusal would land on a
// surface the dialog is covering.
func (a *App) submitComboCreate() {
	d := &a.receiverDialog
	valid := func(key string) bool {
		for _, v := range a.comboCandidates() {
			if v.Key == key {
				return true
			}
		}
		return false
	}
	if !valid(d.comboLeft) || !valid(d.comboRight) {
		d.comboError = locales.ComboErrorPick()
		return
	}
	if d.comboLeft == d.comboRight {
		d.comboError = locales.ComboErrorSame()
		return
	}
	a.eng.LinkReceivers(d.comboLeft, d.comboRight, strings.TrimSpace(d.comboName))
	a.closeReceiverDialog()
}

// receiverComboUnlinkDialog asks before splitting a combo: what splits, and
// that each receiver keeps its own settings.
func (a *App) receiverComboUnlinkDialog(c *ui.Context, p palette) {
	key := a.receiverDialog.key
	v, ok := a.receiverViewFor(key)
	if !ok {
		a.closeReceiverDialog()
		return
	}
	ui.Column(c).Children(func() {
		dialogHead(c, p, iconUnlink, locales.ComboUnlinkTitle(), locales.ComboUnlinkSub(v.Name), true, nil)
		ui.Column(c).Margin(13, 0, 0, 0).Gap(5).Children(func() {
			ui.Text(c, locales.ComboUnlinkHint()).FontSize(11.5).TextColor(p.muted)
		})
		dialogErrorRow(c, p, "")
		dialogFooter(c, p,
			func() { ui.Spacer(c) },
			func() {
				quietButton(c, p, locales.ActionCancel(), func() { a.closeReceiverDialog() })
				primaryDangerButton(c, p, locales.ComboUnlink(), func() {
					delete(a.receiverOpen, key)
					delete(a.mutedVolume, key)
					a.eng.UnlinkCombo(key)
					a.closeReceiverDialog()
				})
			},
		)
	})
}
