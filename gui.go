package main

import (
	"fmt"
	"image"
	"image/color"
	"log"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"gioui.org/app"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/sqweek/dialog"
	"golang.org/x/exp/shiny/materialdesign/icons"
)

func getIcon(data []byte) *widget.Icon {
	ic, _ := widget.NewIcon(data)
	return ic
}

var (
	iconSettings = getIcon(icons.ActionSettings)
	iconPlay     = getIcon(icons.AVPlayArrow)
	iconStop     = getIcon(icons.AVStop)
	iconClean    = getIcon(icons.ActionDelete)
	iconUndo     = getIcon(icons.ContentUndo)
	iconAdd      = getIcon(icons.ContentAdd)
	iconEdit     = getIcon(icons.EditorFormatPaint)
	iconBack     = getIcon(icons.NavigationArrowBack)
	iconNotif    = getIcon(icons.SocialNotifications)
	iconNotifOff = getIcon(icons.SocialNotificationsOff)
	iconStartup  = getIcon(icons.HardwareComputer)
	iconFolder   = getIcon(icons.FileFolder)
	iconClear    = getIcon(icons.ContentClear)
	iconInfo     = getIcon(icons.ActionInfo)

	// Widgets / interaction state
	StartStopBtn        widget.Clickable
	CleanUpBtn          widget.Clickable
	UndoBtn             widget.Clickable
	SettingsBtn         widget.Clickable
	BackBtn             widget.Clickable
	AddFolderBtn        widget.Clickable
	ChangeFileTypesBtn  widget.Clickable
	NotificationsBtn    widget.Clickable
	StartupBtn          widget.Clickable
	DestinationBtn      widget.Clickable
	ClearDestBtn        widget.Clickable
	ResetPathsBtn       widget.Clickable
	ResetMapBtn         widget.Clickable
	addFolderToWatchBtn widget.Clickable
	addTypeBtn          widget.Clickable
	dialogOverlay       widget.Clickable

	extEditor    widget.Editor
	folderEditor widget.Editor

	moveList     widget.List
	pathList     widget.List
	fileTypeList widget.List
	logList      widget.List

	deleteButtons    []widget.Clickable
	mapDeleteButtons []widget.Clickable

	addErrorMsg string

	showSettings        bool
	showFileTypesEditor bool
	isDialogOpen        bool

	// In-memory activity log ring (fed by Org.logCallback)
	activityLog   []string
	activityLogMu sync.Mutex
)

// pushActivityLog appends a line to the in-memory log ring buffer.
func pushActivityLog(msg string) {
	activityLogMu.Lock()
	defer activityLogMu.Unlock()
	activityLog = append(activityLog, msg)
	const cap = 250
	if len(activityLog) > cap {
		activityLog = activityLog[len(activityLog)-cap:]
	}
}

func GuiLoop() {
	for range showGuiCh {
		OpenWindow()
	}
}

func OpenWindow() {
	W := new(app.Window)
	W.Option(
		app.Title("Organization Moderator"),
		app.Size(unit.Dp(1080), unit.Dp(720)),
		app.MinSize(unit.Dp(820), unit.Dp(560)),
	)
	if err := Run(W); err != nil {
		log.Fatal(err)
	}
}

func Run(Window *app.Window) error {
	theme := material.NewTheme()
	theme.Palette.Fg = TextColor
	theme.Palette.ContrastFg = TextColor
	theme.Palette.ContrastBg = PrimaryColor

	var ops op.Ops

	Org.logCallback = func(msg string) {
		pushActivityLog(msg)
		Window.Invalidate()
	}

	for {
		switch e := Window.Event().(type) {
		case app.DestroyEvent:
			return e.Err
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)

			fillShape(gtx.Ops, BackgroundColor, gtx.Constraints.Max)

			handleEvents(gtx, Window)

			drawRoot(gtx, theme)

			// Modal overlay: swallows clicks and dims UI while a native dialog is open.
			if isDialogOpen {
				dialogOverlay.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					paint.FillShape(gtx.Ops,
						color.NRGBA{R: 0, G: 0, B: 0, A: 200},
						clip.Rect{Max: gtx.Constraints.Max}.Op())
					return layout.Dimensions{Size: gtx.Constraints.Max}
				})
			}

			e.Frame(gtx.Ops)
		}
	}
}

// ------------------------------------------------------------------ events

func handleEvents(gtx layout.Context, w *app.Window) {
	if StartStopBtn.Clicked(gtx) {
		if !Org.Config.IsRunning {
			Org.Start()
		} else {
			Org.Stop()
		}
	}
	if CleanUpBtn.Clicked(gtx) {
		if !Org.Config.IsCleaning {
			go Org.ClearNow()
		}
	}
	if UndoBtn.Clicked(gtx) {
		Org.Config.mu.Lock()
		canUndo := len(Org.RecentMoves) > 0 && time.Now().Unix()-Org.RecentMoves[0].UnixTime <= 30
		Org.Config.mu.Unlock()
		if canUndo {
			go Org.Undolastmove()
		}
	}
	if SettingsBtn.Clicked(gtx) {
		showSettings = !showSettings
		showFileTypesEditor = false
	}
	if AddFolderBtn.Clicked(gtx) {
		showSettings = true
		showFileTypesEditor = false
	}
	if BackBtn.Clicked(gtx) {
		showSettings = false
		showFileTypesEditor = false
	}
	if ChangeFileTypesBtn.Clicked(gtx) {
		showSettings = true
		showFileTypesEditor = true
		addErrorMsg = ""
	}
	if NotificationsBtn.Clicked(gtx) {
		Org.SetNotifications(!Org.NotificationsEnabled())
	}
	if StartupBtn.Clicked(gtx) {
		Org.Config.RunAtStartup = !Org.Config.RunAtStartup
		SetAutoStart(Org.Config.RunAtStartup)
		Org.SaveConfigToDisk()
	}
	if DestinationBtn.Clicked(gtx) {
		openFolderPicker(w, "Select Destination Folder", func(dir string) { Org.SetDestination(dir) })
	}
	if ClearDestBtn.Clicked(gtx) {
		Org.SetDestination("")
	}
	if addFolderToWatchBtn.Clicked(gtx) {
		openFolderPicker(w, "Select Folder to Watch", func(dir string) { Org.AddPath(dir) })
	}
	if ResetPathsBtn.Clicked(gtx) {
		Org.ResetWatchPaths()
	}
	if ResetMapBtn.Clicked(gtx) {
		Org.ResetTargetMap()
	}
	if addTypeBtn.Clicked(gtx) {
		ext := extEditor.Text()
		folder := folderEditor.Text()
		if ext != "" && folder != "" {
			if err := Org.AddToMap(ext, folder); err != nil {
				addErrorMsg = err.Error()
			} else {
				addErrorMsg = ""
				extEditor.SetText("")
				folderEditor.SetText("")
			}
		}
	}
}

// openFolderPicker shows a native directory dialog, guarded so it can't be
// re-triggered while one is already up.
func openFolderPicker(w *app.Window, title string, onPick func(string)) {
	if isDialogOpen {
		return
	}
	isDialogOpen = true
	w.Invalidate()
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		time.Sleep(100 * time.Millisecond)
		dir, err := dialog.Directory().Title(title).Browse()
		if err == nil && dir != "" {
			onPick(dir)
		}
		isDialogOpen = false
		w.Invalidate()
	}()
}

// ------------------------------------------------------------------ root

func drawRoot(gtx layout.Context, theme *material.Theme) layout.Dimensions {
	return layout.UniformInset(unit.Dp(10)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return drawBorderedPanel(gtx, BorderColor, unit.Dp(2), unit.Dp(14), SurfaceColor,
			func(gtx layout.Context) layout.Dimensions {
				return layout.UniformInset(unit.Dp(10)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return drawHeaderBar(gtx, theme)
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return drawHLine(gtx, BorderColor)
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),

						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
								layout.Flexed(0.38, func(gtx layout.Context) layout.Dimensions {
									return drawLeftColumn(gtx, theme)
								}),
								layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
								layout.Flexed(0.62, func(gtx layout.Context) layout.Dimensions {
									return drawRightColumn(gtx, theme)
								}),
							)
						}),

						layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return drawHLine(gtx, BorderColor)
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return drawStatusBar(gtx, theme)
						}),
					)
				})
			},
		)
	})
}

func drawHeaderBar(gtx layout.Context, theme *material.Theme) layout.Dimensions {
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min = image.Pt(gtx.Dp(unit.Dp(28)), gtx.Dp(unit.Dp(28)))
			gtx.Constraints.Max = gtx.Constraints.Min
			return iconInfo.Layout(gtx, PrimaryColor)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			lbl := material.H6(theme, "ORG MODERATOR")
			lbl.Color = PrimaryColor
			return lbl.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(14)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return drawStatusPill(gtx, theme)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.E.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				if showSettings {
					lbl := material.Caption(theme, "Settings")
					lbl.Color = SecondaryTextColor
					return layout.Inset{Right: unit.Dp(10)}.Layout(gtx, lbl.Layout)
				}
				return layout.Dimensions{}
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			size := gtx.Dp(unit.Dp(38))
			gtx.Constraints.Min = image.Pt(size, size)
			gtx.Constraints.Max = gtx.Constraints.Min
			btn := material.Button(theme, &SettingsBtn, "")
			btn.Background = SettingsBtnColor
			btn.Inset = layout.Inset{}
			return layout.Stack{Alignment: layout.Center}.Layout(gtx,
				layout.Expanded(btn.Layout),
				layout.Stacked(func(gtx layout.Context) layout.Dimensions {
					inner := gtx.Dp(unit.Dp(18))
					gtx.Constraints.Min = image.Pt(inner, inner)
					gtx.Constraints.Max = gtx.Constraints.Min
					return iconSettings.Layout(gtx, TextColor)
				}),
			)
		}),
	)
}

func drawStatusPill(gtx layout.Context, theme *material.Theme) layout.Dimensions {
	label := "Idle"
	col := SecondaryTextColor
	if Org.Config.IsRunning {
		label = "Running"
		col = SuccessColor
	}
	if Org.Config.IsCleaning {
		label = "Cleaning…"
		col = SettingsBtnColor
	}
	return layout.Stack{Alignment: layout.W}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			size := image.Pt(gtx.Constraints.Min.X, gtx.Constraints.Min.Y)
			rr := gtx.Dp(unit.Dp(10))
			defer clip.RRect{Rect: image.Rectangle{Max: size}, SE: rr, SW: rr, NE: rr, NW: rr}.
				Push(gtx.Ops).Pop()
			paint.ColorOp{Color: withAlpha(col, 40)}.Add(gtx.Ops)
			paint.PaintOp{}.Add(gtx.Ops)
			return layout.Dimensions{Size: size}
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{
				Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(10), Right: unit.Dp(12),
			}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return drawDot(gtx, col, 6)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						lbl := material.Caption(theme, label)
						lbl.Color = col
						return lbl.Layout(gtx)
					}),
				)
			})
		}),
	)
}

// ------------------------------------------------------------------ columns

func drawLeftColumn(gtx layout.Context, theme *material.Theme) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return drawControlsCard(gtx, theme)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return drawSnapshotCard(gtx, theme)
			})
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return drawActivityLogCard(gtx, theme)
		}),
	)
}

func drawRightColumn(gtx layout.Context, theme *material.Theme) layout.Dimensions {
	if showSettings {
		return drawSettingsFullColumn(gtx, theme)
	}
	return drawRecentMovesCard(gtx, theme)
}

// ------------------------------------------------------------------ left: cards

func drawControlsCard(gtx layout.Context, theme *material.Theme) layout.Dimensions {
	return drawBorderedPanel(gtx, BorderColor, unit.Dp(1), unit.Dp(10), InputBgColor,
		func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(unit.Dp(12)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return drawSectionTitle(gtx, theme, "Controls", PrimaryColor)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						btnText := "Start Watching"
						bgColor := SuccessColor
						icn := iconPlay
						if Org.Config.IsRunning {
							btnText = "Stop Watching"
							bgColor = ErrorColor
							icn = iconStop
						}
						gtx.Constraints.Min.X = gtx.Constraints.Max.X
						return styledButton(gtx, theme, &StartStopBtn, icn, btnText, bgColor)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								return layout.Inset{Right: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									gtx.Constraints.Min.X = gtx.Constraints.Max.X
									return styledButton(gtx, theme, &CleanUpBtn, iconClean, "Clean Up", ButtonColor)
								})
							}),
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								return layout.Inset{Left: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									gtx.Constraints.Min.X = gtx.Constraints.Max.X
									bg := DeleteBtnBg
									if canUndoNow() {
										bg = SettingsBtnColor
									}
									return styledButton(gtx, theme, &UndoBtn, iconUndo, "Undo", bg)
								})
							}),
						)
					}),
				)
			})
		})
}

func drawSnapshotCard(gtx layout.Context, theme *material.Theme) layout.Dimensions {
	Org.Config.mu.Lock()
	watchCount := len(Org.Config.WatchPaths)
	dest := Org.Config.DestinationPath
	Org.Config.mu.Unlock()
	Org.Config.Rmu.RLock()
	mapCount := len(Org.Config.TargetMap)
	Org.Config.Rmu.RUnlock()

	return drawBorderedPanel(gtx, BorderColor, unit.Dp(1), unit.Dp(10), InputBgColor,
		func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(unit.Dp(12)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return drawSectionTitle(gtx, theme, "Snapshot", PrimaryColor)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return drawSnapshotRow(gtx, theme, "Folders watched", fmt.Sprintf("%d", watchCount))
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return drawSnapshotRow(gtx, theme, "Extensions mapped", fmt.Sprintf("%d", mapCount))
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						target := "(same folder)"
						if dest != "" {
							target = shortenPath(dest, 30)
						}
						return drawSnapshotRow(gtx, theme, "Destination", target)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						notif := "Off"
						if Org.NotificationsEnabled() {
							notif = "On"
						}
						return drawSnapshotRow(gtx, theme, "Notifications", notif)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						autostart := "Off"
						if Org.Config.RunAtStartup {
							autostart = "On"
						}
						return drawSnapshotRow(gtx, theme, "Start with PC", autostart)
					}),
				)
			})
		})
}

func drawSnapshotRow(gtx layout.Context, theme *material.Theme, label, value string) layout.Dimensions {
	return layout.Inset{Top: unit.Dp(3), Bottom: unit.Dp(3)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				lbl := material.Body2(theme, label)
				lbl.Color = SecondaryTextColor
				lbl.TextSize = unit.Sp(12)
				return lbl.Layout(gtx)
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return layout.E.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					v := material.Body2(theme, value)
					v.Color = TextColor
					v.TextSize = unit.Sp(12)
					return v.Layout(gtx)
				})
			}),
		)
	})
}

func drawActivityLogCard(gtx layout.Context, theme *material.Theme) layout.Dimensions {
	activityLogMu.Lock()
	view := make([]string, len(activityLog))
	copy(view, activityLog)
	activityLogMu.Unlock()

	// newest first for display
	for i, j := 0, len(view)-1; i < j; i, j = i+1, j-1 {
		view[i], view[j] = view[j], view[i]
	}

	return drawBorderedPanel(gtx, BorderColor, unit.Dp(1), unit.Dp(10), InputBgColor,
		func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(unit.Dp(12)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return drawSectionTitle(gtx, theme, "Activity Log", PrimaryColor)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						if len(view) == 0 {
							return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								gtx.Constraints.Min.X = gtx.Constraints.Max.X
								lbl := material.Caption(theme, "No events yet")
								lbl.Color = SecondaryTextColor
								lbl.Alignment = text.Middle
								return lbl.Layout(gtx)
							})
						}
						logList.Axis = layout.Vertical
						return material.List(theme, &logList).Layout(gtx, len(view),
							func(gtx layout.Context, i int) layout.Dimensions {
								return layout.Inset{Bottom: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									gtx.Constraints.Min.X = gtx.Constraints.Max.X
									lbl := material.Caption(theme, view[i])
									lbl.Color = logColorFor(view[i])
									lbl.TextSize = unit.Sp(11)
									return lbl.Layout(gtx)
								})
							})
					}),
				)
			})
		})
}

func logColorFor(line string) color.NRGBA {
	sample := line
	if len(sample) > 80 {
		sample = sample[:80]
	}
	lower := strings.ToLower(sample)
	switch {
	case strings.Contains(lower, "error"):
		return ErrorColor
	case strings.Contains(lower, "moved:"):
		return SuccessColor
	case strings.Contains(lower, "watching"):
		return PrimaryColor
	case strings.Contains(lower, "stopped"), strings.Contains(lower, "starting"):
		return SecondaryTextColor
	default:
		return TextColor
	}
}

// ------------------------------------------------------------------ right: recent moves

func drawRecentMovesCard(gtx layout.Context, theme *material.Theme) layout.Dimensions {
	Org.Config.mu.Lock()
	moves := make([]MoveRecord, len(Org.RecentMoves))
	copy(moves, Org.RecentMoves)
	Org.Config.mu.Unlock()

	return drawBorderedPanel(gtx, BorderColor, unit.Dp(1), unit.Dp(10), InputBgColor,
		func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(unit.Dp(12)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return drawSectionTitleInline(gtx, theme,
									fmt.Sprintf("Recent Moves (%d)", len(moves)), PrimaryColor)
							}),
							layout.Flexed(1, layout.Spacer{Width: unit.Dp(6)}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								btn := material.Button(theme, &UndoBtn, "Undo last")
								btn.Background = SettingsBtnColor
								btn.Color = TextColor
								btn.TextSize = unit.Sp(11)
								btn.Inset = layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(10), Right: unit.Dp(10)}
								if !canUndoNow() {
									btn.Color = SecondaryTextColor
								}
								return btn.Layout(gtx)
							}),
						)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return drawHLine(gtx, BorderColor)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						if len(moves) == 0 {
							return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										gtx.Constraints.Min = image.Pt(gtx.Dp(unit.Dp(36)), gtx.Dp(unit.Dp(36)))
										return iconPlay.Layout(gtx, color.NRGBA{R: 60, G: 80, B: 90, A: 255})
									}),
									layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										gtx.Constraints.Min.X = gtx.Constraints.Max.X
										lbl := material.Body2(theme, "Waiting for file events…")
										lbl.Color = SecondaryTextColor
										lbl.Alignment = text.Middle
										return lbl.Layout(gtx)
									}),
								)
							})
						}
						moveList.Axis = layout.Vertical
						return material.List(theme, &moveList).Layout(gtx, len(moves),
							func(gtx layout.Context, i int) layout.Dimensions {
								return layout.Inset{Bottom: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return drawActivityCard(gtx, theme, moves[i])
								})
							})
					}),
				)
			})
		})
}

func canUndoNow() bool {
	Org.Config.mu.Lock()
	defer Org.Config.mu.Unlock()
	return len(Org.RecentMoves) > 0 && time.Now().Unix()-Org.RecentMoves[0].UnixTime <= 30
}

// ------------------------------------------------------------------ settings

func drawSettingsFullColumn(gtx layout.Context, theme *material.Theme) layout.Dimensions {
	btnHeight := gtx.Dp(unit.Dp(46))

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return drawSectionTitle(gtx, theme, "Settings", PrimaryColor)
			})
		}),

		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(unit.Dp(4)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical, Spacing: layout.SpaceEvenly}.Layout(gtx,

					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceBetween}.Layout(gtx,
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								return layout.Inset{Right: unit.Dp(5)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									gtx.Constraints.Min.X = gtx.Constraints.Max.X
									gtx.Constraints.Min.Y = btnHeight
									gtx.Constraints.Max.Y = btnHeight
									return styledButton(gtx, theme, &AddFolderBtn, iconFolder, "Manage Folders", ButtonColor)
								})
							}),
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								return layout.Inset{Left: unit.Dp(5)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									gtx.Constraints.Min.X = gtx.Constraints.Max.X
									gtx.Constraints.Min.Y = btnHeight
									gtx.Constraints.Max.Y = btnHeight
									return styledButton(gtx, theme, &ChangeFileTypesBtn, iconEdit, "File Types", SettingsBtnColor)
								})
							}),
						)
					}),

					layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),

					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceBetween}.Layout(gtx,
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								return layout.Inset{Right: unit.Dp(5)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									gtx.Constraints.Min.X = gtx.Constraints.Max.X
									gtx.Constraints.Min.Y = btnHeight
									gtx.Constraints.Max.Y = btnHeight
									return styledButton(gtx, theme, &BackBtn, iconBack, "Back", ErrorColor)
								})
							}),
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								return layout.Inset{Left: unit.Dp(5)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									gtx.Constraints.Min.X = gtx.Constraints.Max.X
									gtx.Constraints.Min.Y = btnHeight
									gtx.Constraints.Max.Y = btnHeight
									notiLabel := "Notifications: Off"
									notiBG := SettingsBtnColor
									icn := iconNotifOff
									if Org.Config.Notifications {
										notiLabel = "Notifications: On"
										notiBG = SuccessColor
										icn = iconNotif
									}
									return styledButton(gtx, theme, &NotificationsBtn, icn, notiLabel, notiBG)
								})
							}),
						)
					}),

					layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),

					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.X = gtx.Constraints.Max.X
						gtx.Constraints.Min.Y = btnHeight
						gtx.Constraints.Max.Y = btnHeight
						startupLabel := "Start with PC: OFF"
						startupBG := SettingsBtnColor
						if Org.Config.RunAtStartup {
							startupLabel = "Start with PC: ON"
							startupBG = SuccessColor
						}
						return styledButton(gtx, theme, &StartupBtn, iconStartup, startupLabel, startupBG)
					}),

					layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),

					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceBetween}.Layout(gtx,
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								return layout.Inset{Right: unit.Dp(5)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									gtx.Constraints.Min.X = gtx.Constraints.Max.X
									gtx.Constraints.Min.Y = btnHeight
									gtx.Constraints.Max.Y = btnHeight
									return styledButton(gtx, theme, &DestinationBtn, iconFolder, "Set Destination", ButtonColor)
								})
							}),
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								return layout.Inset{Left: unit.Dp(5)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									gtx.Constraints.Min.X = gtx.Constraints.Max.X
									gtx.Constraints.Min.Y = btnHeight
									gtx.Constraints.Max.Y = btnHeight
									clearBG := SettingsBtnColor
									if Org.Config.DestinationPath != "" {
										clearBG = ErrorColor
									}
									return styledButton(gtx, theme, &ClearDestBtn, iconClear, "Clear Destination", clearBG)
								})
							}),
						)
					}),

					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						destText := "Default (organize inside each watched folder)"
						destColor := SecondaryTextColor
						if Org.Config.DestinationPath != "" {
							destText = shortenPath(Org.Config.DestinationPath, 60)
							destColor = PrimaryColor
						}
						return layout.Inset{Top: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints.Min.X = gtx.Constraints.Max.X
							lbl := material.Caption(theme, destText)
							lbl.Color = destColor
							lbl.Alignment = text.Middle
							return lbl.Layout(gtx)
						})
					}),
				)
			})
		}),

		layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),

		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return drawBorderedPanel(gtx, BorderColor, unit.Dp(1), unit.Dp(8), InputBgColor,
				func(gtx layout.Context) layout.Dimensions {
					if showFileTypesEditor {
						return drawFileTypesEditor(gtx, theme)
					}
					return drawSettingsPanel(gtx, theme)
				},
			)
		}),
	)
}

func drawSettingsPanel(gtx layout.Context, theme *material.Theme) layout.Dimensions {
	Org.Config.mu.Lock()
	paths := make([]string, len(Org.Config.WatchPaths))
	copy(paths, Org.Config.WatchPaths)
	Org.Config.mu.Unlock()

	if len(deleteButtons) != len(paths) {
		deleteButtons = make([]widget.Clickable, len(paths))
	}
	pathList.Axis = layout.Vertical

	return layout.UniformInset(unit.Dp(12)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return drawSectionTitleInline(gtx, theme, "Watching Directories", SettingsBtnColor)
					}),
					layout.Flexed(1, layout.Spacer{Width: unit.Dp(6)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						btn := material.Button(theme, &addFolderToWatchBtn, "Add")
						btn.Background = SuccessColor
						btn.Color = TextColor
						btn.TextSize = unit.Sp(11)
						btn.Inset = layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(10), Right: unit.Dp(10)}
						return btn.Layout(gtx)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						btn := material.Button(theme, &ResetPathsBtn, "Reset")
						btn.Background = SettingsBtnColor
						btn.Color = TextColor
						btn.TextSize = unit.Sp(11)
						btn.Inset = layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(10), Right: unit.Dp(10)}
						return btn.Layout(gtx)
					}),
				)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return drawHLine(gtx, BorderColor)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				if len(paths) == 0 {
					return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.X = gtx.Constraints.Max.X
						lbl := material.Caption(theme, "No folders added yet")
						lbl.Color = SecondaryTextColor
						lbl.Alignment = text.Middle
						return lbl.Layout(gtx)
					})
				}
				return material.List(theme, &pathList).Layout(gtx, len(paths),
					func(gtx layout.Context, i int) layout.Dimensions {
						return layout.Inset{Bottom: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							if deleteButtons[i].Clicked(gtx) {
								Org.Config.mu.Lock()
								Org.Config.WatchPaths = append(Org.Config.WatchPaths[:i], Org.Config.WatchPaths[i+1:]...)
								Org.Config.mu.Unlock()
								Org.SaveConfigToDisk()
							}
							return drawPathCard(gtx, theme, paths[i], &deleteButtons[i])
						})
					})
			}),
		)
	})
}

func drawFileTypesEditor(gtx layout.Context, theme *material.Theme) layout.Dimensions {
	return layout.UniformInset(unit.Dp(12)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return drawSectionTitleInline(gtx, theme, "Extension → Folder", PrimaryColor)
					}),
					layout.Flexed(1, layout.Spacer{Width: unit.Dp(6)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						btn := material.Button(theme, &ResetMapBtn, "Reset")
						btn.Background = SettingsBtnColor
						btn.Color = TextColor
						btn.TextSize = unit.Sp(11)
						btn.Inset = layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(10), Right: unit.Dp(10)}
						return btn.Layout(gtx)
					}),
				)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return drawHLine(gtx, BorderColor)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),

			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
					layout.Flexed(0.35, func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Right: unit.Dp(4)}.Layout(gtx, borderedEditor(theme, &extEditor, "Ext (.ico)"))
					}),
					layout.Flexed(0.35, func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Left: unit.Dp(2), Right: unit.Dp(4)}.Layout(gtx, borderedEditor(theme, &folderEditor, "Folder (Images)"))
					}),
					layout.Flexed(0.30, func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Left: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints.Min.X = gtx.Constraints.Max.X
							return styledButton(gtx, theme, &addTypeBtn, iconAdd, "Add", SuccessColor)
						})
					}),
				)
			}),

			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if addErrorMsg == "" {
					return layout.Dimensions{}
				}
				lbl := material.Caption(theme, addErrorMsg)
				lbl.Color = ErrorColor
				return layout.Inset{Top: unit.Dp(6)}.Layout(gtx, lbl.Layout)
			}),

			layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),

			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				Org.Config.Rmu.RLock()
				keys := make([]string, 0, len(Org.Config.TargetMap))
				folders := make(map[string]string, len(Org.Config.TargetMap))
				for k, v := range Org.Config.TargetMap {
					keys = append(keys, k)
					folders[k] = v
				}
				Org.Config.Rmu.RUnlock()
				sort.Strings(keys)

				if len(mapDeleteButtons) != len(keys) {
					mapDeleteButtons = make([]widget.Clickable, len(keys))
				}
				fileTypeList.Axis = layout.Vertical

				if len(keys) == 0 {
					return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.X = gtx.Constraints.Max.X
						lbl := material.Caption(theme, "No mapping added yet")
						lbl.Color = SecondaryTextColor
						lbl.Alignment = text.Middle
						return lbl.Layout(gtx)
					})
				}
				return material.List(theme, &fileTypeList).Layout(gtx, len(keys),
					func(gtx layout.Context, i int) layout.Dimensions {
						return layout.Inset{Bottom: unit.Dp(5)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							key := keys[i]
							folder := folders[key]
							if mapDeleteButtons[i].Clicked(gtx) {
								Org.RemoveFromMap(key)
							}
							return drawMappingCard(gtx, theme, key, folder, &mapDeleteButtons[i])
						})
					})
			}),
		)
	})
}

func borderedEditor(theme *material.Theme, ed *widget.Editor, hint string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.Y = gtx.Dp(unit.Dp(42))
		gtx.Constraints.Max.Y = gtx.Dp(unit.Dp(42))
		e := material.Editor(theme, ed, hint)
		border := widget.Border{Color: BorderColor, CornerRadius: unit.Dp(6), Width: unit.Dp(1)}
		return border.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(10), Bottom: unit.Dp(10), Left: unit.Dp(10), Right: unit.Dp(10)}.Layout(gtx, e.Layout)
		})
	}
}

// ------------------------------------------------------------------ cards

func drawActivityCard(gtx layout.Context, theme *material.Theme, move MoveRecord) layout.Dimensions {
	accent := categoryColor(move.Dest)
	return layout.Stack{Alignment: layout.W}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			size := image.Pt(gtx.Constraints.Min.X, gtx.Constraints.Min.Y)
			rr := gtx.Dp(unit.Dp(6))
			defer clip.RRect{Rect: image.Rectangle{Max: size}, SE: rr, SW: rr, NE: rr, NW: rr}.
				Push(gtx.Ops).Pop()
			paint.ColorOp{Color: SurfaceColor}.Add(gtx.Ops)
			paint.PaintOp{}.Add(gtx.Ops)

			strip := image.Rectangle{Max: image.Pt(gtx.Dp(unit.Dp(3)), size.Y)}
			defer clip.RRect{Rect: strip, NW: rr, SW: rr}.Push(gtx.Ops).Pop()
			paint.ColorOp{Color: accent}.Add(gtx.Ops)
			paint.PaintOp{}.Add(gtx.Ops)
			return layout.Dimensions{Size: size}
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{
				Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(12), Right: unit.Dp(12),
			}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						lbl := material.Caption(theme, move.Time)
						lbl.Color = accent
						return lbl.Layout(gtx)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						lbl := material.Body2(theme, move.FileName)
						lbl.Color = TextColor
						lbl.TextSize = unit.Sp(12)
						lbl.MaxLines = 1
						return lbl.Layout(gtx)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						lbl := material.Caption(theme, "→ "+move.Dest)
						lbl.Color = SecondaryTextColor
						return lbl.Layout(gtx)
					}),
				)
			})
		}),
	)
}

func drawPathCard(gtx layout.Context, theme *material.Theme, path string, delBtn *widget.Clickable) layout.Dimensions {
	gtx.Constraints.Min.X = gtx.Constraints.Max.X
	return layout.Stack{Alignment: layout.Center}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			size := image.Pt(gtx.Constraints.Min.X, gtx.Constraints.Min.Y)
			rr := gtx.Dp(unit.Dp(6))
			defer clip.RRect{Rect: image.Rectangle{Max: size}, SE: rr, SW: rr, NE: rr, NW: rr}.
				Push(gtx.Ops).Pop()
			paint.ColorOp{Color: SurfaceColor}.Add(gtx.Ops)
			paint.PaintOp{}.Add(gtx.Ops)

			strip := image.Rectangle{Max: image.Pt(gtx.Dp(unit.Dp(3)), size.Y)}
			defer clip.RRect{Rect: strip, NW: rr, SW: rr}.Push(gtx.Ops).Pop()
			paint.ColorOp{Color: PrimaryColor}.Add(gtx.Ops)
			paint.PaintOp{}.Add(gtx.Ops)
			return layout.Dimensions{Size: size}
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{
				Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(12), Right: unit.Dp(8),
			}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min = image.Pt(gtx.Dp(unit.Dp(16)), gtx.Dp(unit.Dp(16)))
						gtx.Constraints.Max = gtx.Constraints.Min
						return iconFolder.Layout(gtx, PrimaryColor)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						lbl := material.Caption(theme, path)
						lbl.Color = TextColor
						lbl.MaxLines = 1
						return lbl.Layout(gtx)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min = image.Pt(gtx.Dp(unit.Dp(26)), gtx.Dp(unit.Dp(26)))
						gtx.Constraints.Max = gtx.Constraints.Min
						btn := material.Button(theme, delBtn, "")
						btn.Background = DeleteBtnBg
						btn.Inset = layout.Inset{}
						return layout.Stack{Alignment: layout.Center}.Layout(gtx,
							layout.Expanded(btn.Layout),
							layout.Stacked(func(gtx layout.Context) layout.Dimensions {
								inner := gtx.Dp(unit.Dp(14))
								gtx.Constraints.Min = image.Pt(inner, inner)
								gtx.Constraints.Max = gtx.Constraints.Min
								return iconClear.Layout(gtx, ErrorColor)
							}),
						)
					}),
				)
			})
		}),
	)
}

func drawMappingCard(gtx layout.Context, theme *material.Theme, ext string, folder string, delBtn *widget.Clickable) layout.Dimensions {
	gtx.Constraints.Min.X = gtx.Constraints.Max.X
	accent := categoryColor(folder)
	return layout.Stack{Alignment: layout.Center}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			size := image.Pt(gtx.Constraints.Min.X, gtx.Constraints.Min.Y)
			rr := gtx.Dp(unit.Dp(6))
			defer clip.RRect{Rect: image.Rectangle{Max: size}, SE: rr, SW: rr, NE: rr, NW: rr}.
				Push(gtx.Ops).Pop()
			paint.ColorOp{Color: SurfaceColor}.Add(gtx.Ops)
			paint.PaintOp{}.Add(gtx.Ops)

			strip := image.Rectangle{Max: image.Pt(gtx.Dp(unit.Dp(3)), size.Y)}
			defer clip.RRect{Rect: strip, NW: rr, SW: rr}.Push(gtx.Ops).Pop()
			paint.ColorOp{Color: accent}.Add(gtx.Ops)
			paint.PaintOp{}.Add(gtx.Ops)
			return layout.Dimensions{Size: size}
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{
				Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(12), Right: unit.Dp(8),
			}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.Y = gtx.Dp(unit.Dp(28))
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						txt := fmt.Sprintf("%s  →  %s", ext, folder)
						lbl := material.Caption(theme, txt)
						lbl.Color = TextColor
						return lbl.Layout(gtx)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min = image.Pt(gtx.Dp(unit.Dp(24)), gtx.Dp(unit.Dp(24)))
						gtx.Constraints.Max = gtx.Constraints.Min
						btn := material.Button(theme, delBtn, "")
						btn.Background = DeleteBtnBg
						btn.Inset = layout.Inset{}
						return layout.Stack{Alignment: layout.Center}.Layout(gtx,
							layout.Expanded(btn.Layout),
							layout.Stacked(func(gtx layout.Context) layout.Dimensions {
								inner := gtx.Dp(unit.Dp(13))
								gtx.Constraints.Min = image.Pt(inner, inner)
								gtx.Constraints.Max = gtx.Constraints.Min
								return iconClear.Layout(gtx, ErrorColor)
							}),
						)
					}),
				)
			})
		}),
	)
}

// ------------------------------------------------------------------ bottom status

func drawStatusBar(gtx layout.Context, theme *material.Theme) layout.Dimensions {
	status := "Idle — press Start to begin"
	statusColor := SecondaryTextColor
	if Org.Config.IsRunning {
		status = "Running — watching for changes"
		statusColor = SuccessColor
	}
	if Org.Config.IsCleaning {
		status = "Cleaning existing files…"
		statusColor = SettingsBtnColor
	}
	return layout.Inset{
		Top: unit.Dp(2), Bottom: unit.Dp(2), Left: unit.Dp(4), Right: unit.Dp(4),
	}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		dims := layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return drawDot(gtx, statusColor, 6)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				lbl := material.Caption(theme, status)
				lbl.Color = statusColor
				return lbl.Layout(gtx)
			}),
		)
		dims.Size.X = gtx.Constraints.Max.X
		return dims
	})
}

// ------------------------------------------------------------------ generic widgets

func styledButton(gtx layout.Context, theme *material.Theme, btn *widget.Clickable, icn *widget.Icon, label string, bg color.NRGBA) layout.Dimensions {
	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			rr := gtx.Dp(unit.Dp(8))
			rect := image.Rectangle{Max: gtx.Constraints.Min}
			bgColor := bg
			if btn.Hovered() {
				bgColor = addAlpha(bg, 230)
			}
			if btn.Pressed() {
				bgColor = addAlpha(bg, 255)
			}
			defer clip.RRect{Rect: rect, SE: rr, SW: rr, NE: rr, NW: rr}.Push(gtx.Ops).Pop()
			paint.ColorOp{Color: bgColor}.Add(gtx.Ops)
			paint.PaintOp{}.Add(gtx.Ops)
			return layout.Dimensions{Size: gtx.Constraints.Min}
		}),

		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			bw := gtx.Dp(unit.Dp(4))
			rr := gtx.Dp(unit.Dp(8))
			rect := image.Rectangle{Max: image.Pt(bw, gtx.Constraints.Min.Y)}
			defer clip.RRect{Rect: rect, NW: rr, SW: rr}.Push(gtx.Ops).Pop()
			paint.ColorOp{Color: color.NRGBA{R: 255, G: 255, B: 255, A: 60}}.Add(gtx.Ops)
			paint.PaintOp{}.Add(gtx.Ops)
			return layout.Dimensions{Size: gtx.Constraints.Min}
		}),

		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{
					Top: unit.Dp(10), Bottom: unit.Dp(10),
					Left: unit.Dp(14), Right: unit.Dp(12),
				}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					if icn != nil {
						return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								sz := gtx.Dp(unit.Dp(16))
								gtx.Constraints.Min = image.Pt(sz, sz)
								gtx.Constraints.Max = gtx.Constraints.Min
								return icn.Layout(gtx, TextColor)
							}),
							layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								lbl := material.Body1(theme, label)
								lbl.Color = TextColor
								lbl.TextSize = unit.Sp(13)
								lbl.Alignment = text.Middle
								lbl.MaxLines = 1
								return lbl.Layout(gtx)
							}),
						)
					}
					return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						lbl := material.Body1(theme, label)
						lbl.Color = TextColor
						lbl.TextSize = unit.Sp(13)
						lbl.Alignment = text.Middle
						return lbl.Layout(gtx)
					})
				})
			})
		}),
	)
}

func drawSectionTitle(gtx layout.Context, theme *material.Theme, title string, col color.NRGBA) layout.Dimensions {
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return drawDot(gtx, col, 6)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			lbl := material.H6(theme, title)
			lbl.Color = col
			lbl.TextSize = unit.Sp(14)
			return lbl.Layout(gtx)
		}),
	)
}

func drawSectionTitleInline(gtx layout.Context, theme *material.Theme, title string, col color.NRGBA) layout.Dimensions {
	lbl := material.H6(theme, title)
	lbl.Color = col
	lbl.TextSize = unit.Sp(14)
	return lbl.Layout(gtx)
}

func drawBorderedPanel(gtx layout.Context, borderColor color.NRGBA, borderWidth unit.Dp, radius unit.Dp, fillColor color.NRGBA, w layout.Widget) layout.Dimensions {
	bw := gtx.Dp(borderWidth)
	rr := gtx.Dp(radius)

	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			size := gtx.Constraints.Min
			outerRect := image.Rectangle{Max: size}
			defer clip.RRect{Rect: outerRect, SE: rr, SW: rr, NE: rr, NW: rr}.Push(gtx.Ops).Pop()
			paint.ColorOp{Color: borderColor}.Add(gtx.Ops)
			paint.PaintOp{}.Add(gtx.Ops)
			return layout.Dimensions{Size: size}
		}),

		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			size := gtx.Constraints.Min
			innerRR := rr - bw
			if innerRR < 0 {
				innerRR = 0
			}
			innerRect := image.Rectangle{
				Min: image.Pt(bw, bw),
				Max: image.Pt(size.X-bw, size.Y-bw),
			}
			defer clip.RRect{Rect: innerRect, SE: innerRR, SW: innerRR, NE: innerRR, NW: innerRR}.
				Push(gtx.Ops).Pop()
			paint.ColorOp{Color: fillColor}.Add(gtx.Ops)
			paint.PaintOp{}.Add(gtx.Ops)
			return layout.Dimensions{Size: size}
		}),

		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(borderWidth).Layout(gtx, w)
		}),
	)
}

func fillShape(ops *op.Ops, c color.NRGBA, size image.Point) {
	defer clip.Rect{Max: size}.Push(ops).Pop()
	paint.ColorOp{Color: c}.Add(ops)
	paint.PaintOp{}.Add(ops)
}

func drawHLine(gtx layout.Context, c color.NRGBA) layout.Dimensions {
	h := gtx.Dp(unit.Dp(1))
	rect := image.Rectangle{Max: image.Pt(gtx.Constraints.Max.X, h)}
	defer clip.Rect(rect).Push(gtx.Ops).Pop()
	paint.ColorOp{Color: c}.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
	return layout.Dimensions{Size: rect.Max}
}

func drawDot(gtx layout.Context, c color.NRGBA, dp unit.Dp) layout.Dimensions {
	sz := gtx.Dp(dp)
	dot := image.Rectangle{Max: image.Pt(sz, sz)}
	defer clip.Ellipse{Max: dot.Max}.Push(gtx.Ops).Pop()
	paint.ColorOp{Color: c}.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
	return layout.Dimensions{Size: image.Pt(sz+4, sz)}
}

func addAlpha(c color.NRGBA, a uint8) color.NRGBA {
	return color.NRGBA{R: c.R, G: c.G, B: c.B, A: a}
}

func withAlpha(c color.NRGBA, a uint8) color.NRGBA {
	return color.NRGBA{R: c.R, G: c.G, B: c.B, A: a}
}

// categoryColor picks a stable accent color for a destination folder name.
func categoryColor(dest string) color.NRGBA {
	switch dest {
	case "Image":
		return color.NRGBA{R: 255, G: 152, B: 0, A: 255}
	case "Video":
		return color.NRGBA{R: 171, G: 71, B: 188, A: 255}
	case "Document":
		return color.NRGBA{R: 3, G: 169, B: 244, A: 255}
	case "Archive":
		return color.NRGBA{R: 141, G: 110, B: 99, A: 255}
	case "Programs":
		return color.NRGBA{R: 0, G: 200, B: 83, A: 255}
	default:
		return PrimaryColor
	}
}

// shortenPath trims long paths for display without breaking words in the middle.
func shortenPath(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 6 {
		return s[:max]
	}
	return "…" + s[len(s)-(max-1):]
}
