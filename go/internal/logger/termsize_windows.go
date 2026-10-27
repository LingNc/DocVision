//go:build windows

package logger

// terminalRows is a no-op on Windows: the console window buffer is not the
// same thing as the visible screen height, so the live block falls back to
// defaultTermRows there instead of guessing wrong.
func terminalRows() int { return 0 }
