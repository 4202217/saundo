//go:build !preview

package app

// act runs the real closure. The shipped build has no preview seam.
func (a *App) act(_ Action, _ any, real func()) {
	real()
}
