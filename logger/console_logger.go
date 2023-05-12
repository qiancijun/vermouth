package logger

import (
	"errors"
	"io"
	"os"
	"sync"

	"github.com/fatih/color"
	"github.com/qiancijun/vermouth/config"
)

const (
	CONSOLE_ADAPTER_NAME = "console"
)

// color 用于设置打印到控制台是否显示颜色
// Format 格式化输出
type ConsoleAdapter struct {
	sync.Mutex
	writer io.Writer
	config config.LogArgs
}

var _ iLogger = (*ConsoleAdapter)(nil)
var levelColors = map[LogLevel]color.Attribute{
	TRACE: color.FgWhite,
	DEBUG: color.FgGreen,
	INFO:  color.FgBlue,
	WARN:  color.FgYellow,
	ERROR: color.FgRed,
	FATAL: color.FgMagenta,
}

func init() {
	Register(CONSOLE_ADAPTER_NAME, NewConsoleAdapter)
}

func NewConsoleAdapter() iLogger {
	return &ConsoleAdapter{
		writer: os.Stdout,
	}
}

func (console *ConsoleAdapter) Init(args config.LogArgs) error {
	if args.Type != CONSOLE_ADAPTER_NAME {
		return errors.New("logger console adapter init error, config must ConsoleConfig")
	}
	console.config = args

	if console.config.Format == "" {
		console.config.Format = defaultLoggerMessageFormat
	}

	return nil
}

func (console *ConsoleAdapter) Name() string {
	return CONSOLE_ADAPTER_NAME
}

func (console *ConsoleAdapter) writeMsg(msg *message) error {
	m := messageFormat(console.config.Format, msg)
	
	writer := console.writer
	if console.config.Color {
		colorAttr := console.getColorByLevel(msg.Level)
		console.Lock()
		color.New(colorAttr).SetWriter(writer).Println(m)
		console.Unlock()
		return nil
	}
	console.Lock()
	writer.Write([]byte(m + "\n"))
	console.Unlock()
	return nil
}

func (console *ConsoleAdapter) Flush() {

}

func (console *ConsoleAdapter) getColorByLevel(level LogLevel) color.Attribute {
	lc, ok := levelColors[level]
	if !ok {
		lc = color.FgWhite
	}
	return lc
}