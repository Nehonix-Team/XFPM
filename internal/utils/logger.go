package utils

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/pterm/pterm"
	"github.com/pterm/pterm/putils"
)

var (
	InfoColor    = pterm.NewStyle(pterm.FgCyan)
	SuccessColor = pterm.NewStyle(pterm.FgGreen, pterm.Bold)
	WarnColor    = pterm.NewStyle(pterm.FgYellow)
	ErrorColor   = pterm.NewStyle(pterm.FgRed, pterm.Bold)
	DimColor     = pterm.NewStyle(pterm.FgGray)
	AccentColor  = pterm.NewStyle(pterm.FgMagenta, pterm.Bold)
	MatrixColor  = pterm.NewStyle(pterm.FgLightGreen)
	GreenColor   = pterm.NewStyle(pterm.FgGreen)
)

func Info(format string, a ...any) {
	printer := pterm.PrefixPrinter{
		Prefix: pterm.Prefix{
			Text:  " [*] ",
			Style: InfoColor,
		},
		MessageStyle: InfoColor,
	}
	printer.Printf(format, a...)
}

func Success(format string, a ...any) {
	printer := pterm.PrefixPrinter{
		Prefix: pterm.Prefix{
			Text:  " [✔] ",
			Style: SuccessColor,
		},
		MessageStyle: SuccessColor,
	}
	printer.Printf(format, a...)
}

func Warn(format string, a ...any) {
	printer := pterm.PrefixPrinter{
		Prefix: pterm.Prefix{
			Text:  " [!] ",
			Style: WarnColor,
		},
		MessageStyle: WarnColor,
	}
	printer.Printf(format, a...)
}

func Error(format string, a ...any) {
	printer := pterm.PrefixPrinter{
		Prefix: pterm.Prefix{
			Text:  " [✘] ",
			Style: ErrorColor,
		},
		MessageStyle: ErrorColor,
	}
	printer.Printf(format, a...)
}

func Log(prefix, msg string) {
	pterm.Printf("   %s %s\n", DimColor.Sprint(prefix), msg)
}

func LiveLog(msg string) {
	pterm.Printf("   %s %s\n", GreenColor.Sprint("→"), msg)
}

func Matrix(msg string) {
	MatrixColor.Printf(" [NEURAL_LINK] %s\n", msg)
}

func Premium(title, msg string) {
	AccentColor.Printf("  ➤ %-12s ", title)
	fmt.Printf("%s\n", msg)
}

func RandomMatrixString(length int) string {
	chars := "010101"
	result := make([]byte, length)
	for i := 0; i < length; i++ {
		result[i] = chars[time.Now().UnixNano()%int64(len(chars))]
	}
	return string(result)
}

func MatrixSimulation(msg string, duration time.Duration) {
	MatrixColor.Printf("  [INIT_SEQ] %s...\n", msg)
	chars := "01010101010101010101"
	steps := 10
	sleep := duration / time.Duration(steps)

	for i := 0; i < steps; i++ {
		fmt.Printf("\r  %s %s", MatrixColor.Sprint("[MATRIX]"), DimColor.Sprint(chars[:(i*2)]))
		time.Sleep(sleep)
	}
	fmt.Printf("\r  %s %s - [COMPLETED]\n", MatrixColor.Sprint("[MATRIX]"), strings.Repeat("1", 20))
}

func StickyStatus(current, status string) {
	pterm.Printf("   %s %-30s %s\n",
		GreenColor.Sprint("├─"),
		current,
		DimColor.Sprint(status),
	)
}

func PrintBanner() {
	pterm.DefaultBigText.WithLetters(
		putils.LettersFromStringWithStyle("XFPM", AccentColor),
	).Render()
	pterm.Printf("  %s %s\n", DimColor.Sprint("Official XyPriss Package Manager"), AccentColor.Sprint("v"+BinVersion))
}

func PrintFooter(duration time.Duration) {
	fmt.Println()
	pterm.Printf("  %s %s\n", 
		SuccessColor.Sprint("Done in"), 
		pterm.FgYellow.Sprint(duration.Round(time.Millisecond)),
	)
	pterm.Printf("  %s %s\n", 
		DimColor.Sprint("Powered by"), 
		pterm.FgLightBlue.Sprint("Nehonix™ & XyPriss Engine"),
	)
}

func SetupUI() {
	pterm.EnableColor()
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		fmt.Printf("\n\033[?25h")
		os.Exit(130)
	}()
}