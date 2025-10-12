// Package ansi provides ANSI escape codes for terminal text formatting.
package ansi

// Reset and formatting.
const (
	Reset     = "\u001b[0m"
	Bold      = "\u001b[1m"
	Dim       = "\u001b[2m"
	Italic    = "\u001b[3m"
	Underline = "\u001b[4m"
	Blink     = "\u001b[5m"
	Reverse   = "\u001b[7m"
	Strike    = "\u001b[9m"
)

// Foreground colours.
const (
	Red     = "\u001b[31m"
	Green   = "\u001b[32m"
	Yellow  = "\u001b[33m"
	Blue    = "\u001b[34m"
	Magenta = "\u001b[35m"
	Cyan    = "\u001b[36m"
	Gray    = "\u001b[37m"
	White   = "\u001b[97m"
)

// Bright foreground colours.
const (
	BrightRed     = "\u001b[91m"
	BrightGreen   = "\u001b[92m"
	BrightYellow  = "\u001b[93m"
	BrightBlue    = "\u001b[94m"
	BrightMagenta = "\u001b[95m"
	BrightCyan    = "\u001b[96m"
	BrightGray    = "\u001b[90m"
	BrightWhite   = White
)

// Background colours.
const (
	BgRed     = "\u001b[41m"
	BgGreen   = "\u001b[42m"
	BgYellow  = "\u001b[43m"
	BgBlue    = "\u001b[44m"
	BgMagenta = "\u001b[45m"
	BgCyan    = "\u001b[46m"
	BgGray    = "\u001b[47m"
	BgWhite   = "\u001b[107m"
)

// Bright background colours.
const (
	BgBrightRed     = "\u001b[101m"
	BgBrightGreen   = "\u001b[102m"
	BgBrightYellow  = "\u001b[103m"
	BgBrightBlue    = "\u001b[104m"
	BgBrightMagenta = "\u001b[105m"
	BgBrightCyan    = "\u001b[106m"
	BgBrightGray    = "\u001b[100m"
	BgBrightWhite   = BgWhite
)
