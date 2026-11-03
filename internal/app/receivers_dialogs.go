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
	ui.DialogBase(c, &a.receiverDialogOpen, func(back, panel ui.Element) {
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
		}
	})
}

// dialogHead is the icon-plus-title head the shells share.
func dialogHead(c *ui.Context, p palette, icon *ui.SVG, title, sub string, danger bool) {
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
		ui.Text(c, label).FontSize(11).FontWeight(600).LetterSpacing(0.2).TextColor(p.muted)
		if hint != "" {
			ui.Text(c, hint).FontSize(10.5).TextColor(p.faint).SingleLine().Ellipsis("…")
		}
	})
}

// dialogInput is the dialog's 32-DIP text field.
func dialogInput(c *ui.Context, p palette, value *string, placeholder, label string) ui.Element {
	in := ui.TextInput(c, value).FillWidth().Height(32).Radius(8).PaddingX(10).
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
		dialogHead(c, p, headIcon, title, sub, false)
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
		dialogHead(c, p, iconKey, title, locales.ReceiverPasswordDialogSub(v.Name), false)
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
			locales.ReceiverRemoveConfirmSub(v.Name, model), true)
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
	if b.Hovered() {
		b.Background(p.redSoft)
	}
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
	if b.Hovered() {
		b.Background(p.red.Mix(p.bg, 0.12))
	}
	b.OnClick(fn)
	b.Children(func() {
		ui.Text(c, label).FontSize(13).FontWeight(500).TextColor(p.bg).SingleLine()
	})
}
