package tui

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func formatDuration(d *durationpb.Duration) string {
	if d == nil {
		return "-"
	}
	return formatDurationValue(d.AsDuration())
}

func formatDurationValue(dur time.Duration) string {
	switch {
	case dur < time.Millisecond:
		return fmt.Sprintf("%.0fµs", float64(dur.Microseconds()))
	case dur < time.Second:
		return fmt.Sprintf("%.1fms", float64(dur.Microseconds())/1000)
	default:
		return fmt.Sprintf("%.2fs", dur.Seconds())
	}
}

func formatTime(t *timestamppb.Timestamp) string {
	if t == nil {
		return "-"
	}
	return t.AsTime().In(time.Local).Format("15:04:05.000") //nolint:gosmopolitan
}

func truncate(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 1 {
		return s[:maxLen]
	}
	return s[:maxLen-1] + "…"
}

func padRight(s string, width int) string {
	w := lipgloss.Width(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}

func padLeft(s string, width int) string {
	w := lipgloss.Width(s)
	if w >= width {
		return s
	}
	return strings.Repeat(" ", width-w) + s
}

func friendlyError(err error, width int) string {
	msg := err.Error()

	var text string
	switch {
	case strings.Contains(msg, "connection refused"),
		strings.Contains(msg, "Unavailable"):
		text = "Could not connect to http-tapd.\n" +
			"Is http-tapd running?\n\n" +
			"Error: " + msg
	default:
		text = "Error: " + msg
	}

	return lipgloss.NewStyle().Width(width).Render(text)
}

func statusStyle(status int32) lipgloss.Style {
	switch {
	case status == 0:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("245")) // gray for unknown
	case status < 300:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("2")) // green
	case status < 400:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("4")) // blue
	case status < 500:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("3")) // yellow
	default:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("1")) // red
	}
}

func statusString(status int32) string {
	if status == 0 {
		return "ERR"
	}
	text := http.StatusText(int(status))
	if text == "" {
		return strconv.Itoa(int(status))
	}
	return fmt.Sprintf("%d %s", status, text)
}

func methodStyle(method string) lipgloss.Style {
	switch method {
	case http.MethodGet:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("2")) // green
	case http.MethodPost:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("4")) // blue
	case http.MethodPut:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("3")) // yellow
	case http.MethodDelete:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("1")) // red
	case http.MethodPatch:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("5")) // magenta
	default:
		return lipgloss.NewStyle()
	}
}

func formatBody(data []byte) []string {
	if lines := tryFormatJSON(data); lines != nil {
		return lines
	}
	if utf8.Valid(data) {
		s := strings.TrimSpace(string(data))
		return strings.Split(s, "\n")
	}
	dump := hex.Dump(data)
	return strings.Split(strings.TrimRight(dump, "\n"), "\n")
}

func tryFormatJSON(data []byte) []string {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return nil
	}
	pretty, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil
	}
	return strings.Split(string(pretty), "\n")
}

func formatHeaders(headers map[string]string) []string {
	keys := make([]string, 0, len(headers))
	for k := range headers {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	lines := make([]string, 0, len(keys))
	for _, k := range keys {
		lines = append(lines, k+": "+headers[k])
	}
	return lines
}

func overlayAlert(bg, msg string, width int) string {
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("2")).
		Padding(0, 2).
		Render(msg)

	fgLines := strings.Split(box, "\n")
	bgLines := strings.Split(bg, "\n")

	startY := max((len(bgLines)-len(fgLines))/2, 0)
	for i, fl := range fgLines {
		y := startY + i
		if y >= len(bgLines) {
			break
		}
		fw := lipgloss.Width(fl)
		pad := max((width-fw)/2, 0)
		left := ansi.Cut(bgLines[y], 0, pad)
		right := ansi.Cut(bgLines[y], pad+fw, width)
		bgLines[y] = left + fl + right
	}
	return strings.Join(bgLines, "\n")
}
