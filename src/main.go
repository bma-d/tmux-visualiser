package main

import (
	"context"
	"flag"
	"fmt"
	"time"

	"github.com/gdamore/tcell/v2"
)

func main() {
	cfg := config{}
	flag.IntVar(&cfg.lines, "lines", 500, "number of lines to capture per session")
	flag.DurationVar(&cfg.interval, "interval", 1*time.Second, "refresh interval")
	flag.DurationVar(&cfg.cmdTimeout, "cmd-timeout", 900*time.Millisecond, "timeout for each tmux command")
	flag.IntVar(&cfg.maxWorkers, "workers", 4, "max concurrent tmux capture workers")
	flag.Parse()

	if cfg.lines < 20 {
		cfg.lines = 20
	}
	if cfg.interval < 200*time.Millisecond {
		cfg.interval = 200 * time.Millisecond
	}
	if cfg.cmdTimeout < 300*time.Millisecond {
		cfg.cmdTimeout = 300 * time.Millisecond
	}
	if cfg.maxWorkers < 1 {
		cfg.maxWorkers = 1
	}

	screen, err := tcell.NewScreen()
	if err != nil {
		fmt.Println("failed to create screen:", err)
		return
	}
	if err := screen.Init(); err != nil {
		fmt.Println("failed to init screen:", err)
		return
	}
	defer screen.Fini()

	screen.EnableMouse()

	state := appState{sessions: map[string]sessionView{}, scroll: map[string]int{}, follow: map[string]bool{}, mouseEnabled: true}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events := make(chan tcell.Event, 16)
	go func() {
		for {
			ev := screen.PollEvent()
			if ev == nil {
				close(events)
				return
			}
			events <- ev
		}
	}()

	refresh := func() {
		updateState(ctx, &state, cfg)
		draw(screen, state, cfg)
	}

	refresh()
	ticker := time.NewTicker(cfg.interval)
	defer ticker.Stop()
	resetTicker := func() {
		ticker.Stop()
		ticker = time.NewTicker(cfg.interval)
	}

	running := true
	for running {
		select {
		case <-ticker.C:
			refresh()
		case ev, ok := <-events:
			if !ok {
				running = false
				break
			}
			switch tev := ev.(type) {
			case *tcell.EventResize:
				screen.Sync()
				draw(screen, state, cfg)
			case *tcell.EventKey:
				if state.composeActive {
					if handleComposeKey(ctx, &state, cfg, tev) {
						draw(screen, state, cfg)
					}
					continue
				}
				if state.selectTarget {
					if handleSelectKey(ctx, &state, cfg, tev, screen) {
						draw(screen, state, cfg)
					}
					continue
				}
				switch tev.Key() {
				case tcell.KeyCtrlC:
					running = false
				case tcell.KeyCtrlK:
					if err := killFocusedSession(ctx, &state, cfg); err != nil {
						state.lastErr = err.Error()
					}
					refresh()
				case tcell.KeyUp:
					scrollFocused(&state, screen, -1)
					draw(screen, state, cfg)
				case tcell.KeyDown:
					scrollFocused(&state, screen, 1)
					draw(screen, state, cfg)
				case tcell.KeyPgUp:
					scrollFocused(&state, screen, -5)
					draw(screen, state, cfg)
				case tcell.KeyPgDn:
					scrollFocused(&state, screen, 5)
					draw(screen, state, cfg)
				case tcell.KeyHome:
					jumpScroll(&state, screen, true)
					draw(screen, state, cfg)
				case tcell.KeyEnd:
					jumpScroll(&state, screen, false)
					draw(screen, state, cfg)
				case tcell.KeyTAB:
					moveFocus(&state, 1)
					draw(screen, state, cfg)
				case tcell.KeyBacktab:
					moveFocus(&state, -1)
					draw(screen, state, cfg)
				default:
					switch tev.Rune() {
					case 'q', 'Q':
						running = false
					case 'r', 'R':
						refresh()
					case '+':
						cfg.lines += 50
						refresh()
					case '-':
						cfg.lines -= 50
						if cfg.lines < 20 {
							cfg.lines = 20
						}
						refresh()
					case '[':
						cfg.interval -= 200 * time.Millisecond
						if cfg.interval < 200*time.Millisecond {
							cfg.interval = 200 * time.Millisecond
						}
						resetTicker()
						refresh()
					case ']':
						cfg.interval += 200 * time.Millisecond
						resetTicker()
						refresh()
					case 'i', 'I':
						startCompose(&state)
						draw(screen, state, cfg)
					case 'm', 'M':
						if state.mouseEnabled {
							screen.DisableMouse()
							state.mouseEnabled = false
						} else {
							screen.EnableMouse()
							state.mouseEnabled = true
						}
						draw(screen, state, cfg)
					case 'j', 'J':
						scrollFocused(&state, screen, 1)
						draw(screen, state, cfg)
					case 'k', 'K':
						scrollFocused(&state, screen, -1)
						draw(screen, state, cfg)
					case 'n', 'N':
						moveFocus(&state, 1)
						draw(screen, state, cfg)
					case 'p', 'P':
						moveFocus(&state, -1)
						draw(screen, state, cfg)
					}
				}
			case *tcell.EventMouse:
				if state.selectTarget {
					if handleSelectMouse(ctx, &state, cfg, tev, screen) {
						draw(screen, state, cfg)
					}
					continue
				}
				if state.composeActive {
					continue
				}
				buttons := tev.Buttons()
				if buttons&(tcell.WheelUp|tcell.Button4) != 0 {
					scrollFocused(&state, screen, -3)
					draw(screen, state, cfg)
					continue
				}
				if buttons&(tcell.WheelDown|tcell.Button5) != 0 {
					scrollFocused(&state, screen, 3)
					draw(screen, state, cfg)
					continue
				}
			}
		}
	}
}
