/**
* 基本的 Log 日志都是通过 RootLog 写出的，如果需要配置多输出流，例如把日志写入文件中
* name 就设置为 root。但是为了整合别的框架，也可以配置其他 name 的 Log
* 例如 Zookeeper 就单独使用 name 为 zk 的 Log
* raft 框架的 logger 没有适配，这后续可能会考虑进去，raft 的 logger 太复杂了
 */
package logger

import (
	"fmt"
	"os"
	"path"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/qiancijun/vermouth/config"
	"github.com/spf13/viper"
)

type LogLevel int

const (
	TRACE LogLevel = iota
	DEBUG
	INFO
	WARN
	ERROR
	FATAL
)

type message struct {
	Timestamp         int64    `json:"timestamp"`
	TimestampFormat   string   `json:"timestamp_format"`
	Millisecond       int64    `json:"millisecond"`
	MillisecondFormat string   `json:"millisecond_format"`
	Level             LogLevel `json:"level"`
	LevelString       string   `json:"level_string"`
	Body              string   `json:"body"`
	File              string   `json:"file"`
	Line              int      `json:"line"`
	Function          string   `json:"function"`
}

type iLogger interface {
	Name() string
	Init(config.LogArgs) error
	writeMsg(*message) error
	Flush()
}

type adapterLoggerFunc func() iLogger

type outputLogger struct {
	logType string
	level   LogLevel
	logger  iLogger
}

type Logger struct {
	sync.Mutex
	name        string
	outputs     []*outputLogger
	synchronous bool
	msgChan     chan *message
	wg          sync.WaitGroup
	signalChan  chan string
}

var (
	adapters           = make(map[string]adapterLoggerFunc)
	levelStringMapping = map[LogLevel]string{
		TRACE: "Trace",
		DEBUG: "Debug",
		INFO:  "Info",
		WARN:  "Warn",
		ERROR: "Error",
		FATAL: "Fatal",
	}
	levelIntMapping = map[string]LogLevel{
		"trace": TRACE,
		"debug": DEBUG,
		"info":  INFO,
		"warn":  WARN,
		"error": ERROR,
		"fatal": FATAL,
	}
	defaultLoggerMessageFormat = "[%level_string%] %timestamp_format% %body%"
	Log                        *Logger
	globalLevel                LogLevel
)

var (
	Logs = make(map[string]*Logger)
)

func init() {
	option := config.LogArgs{
		Name:   "root",
		Type:   "console",
		Color:  true,
		Format: "",
		Level:  "info",
	}
	Log = NewLogger()
	Log.Attach("console", INFO, option)
}

func Register(name string, logger adapterLoggerFunc) {
	if _, ok := adapters[name]; ok {
		panic("logger: logger adapter " + name + " already registered!")
	}
	if logger == nil {
		panic("logger: logger adapter " + name + " is nil!")
	}
	adapters[name] = logger
}

func NewLogger() *Logger {
	logger := &Logger{
		outputs:     make([]*outputLogger, 0),
		synchronous: true,
		signalChan:  make(chan string),
		wg:          sync.WaitGroup{},
		msgChan:     make(chan *message, 10),
	}
	return logger
}

// 从配置文件中读取日志配置信息
func BuildFromConfig(args *config.VermouthConfig) {
	for _, logArgs := range args.Log {
		logType, level, name := logArgs.Type, logArgs.Level, logArgs.Name
		log, ok := Logs[name]
		if !ok {
			log = NewLogger()
			log.name = name
			Logs[name] = log
		}
		log.Attach(logType, levelIntMapping[level], logArgs)
	}
	rootLog, ok := Logs["root"]
	if ok {
		Log = rootLog
	}
}

// 为 Logger 添加侧输出流
func (l *Logger) Attach(logType string, level LogLevel, args config.LogArgs) error {
	l.Lock()
	defer l.Unlock()
	return l.attach(logType, level, args)
}

func (l *Logger) attach(logType string, level LogLevel, args config.LogArgs) error {
	adapterFunc, ok := adapters[logType]
	if !ok {
		panic("logger: unsupport logger type!")
	}
	newLogger := adapterFunc()
	if err := newLogger.Init(args); err != nil {
		panic("logger: adapter " + l.name + " init failed, error: " + err.Error())
	}
	output := &outputLogger{
		logType: logType,
		level:   level,
		logger:  newLogger,
	}
	l.outputs = append(l.outputs, output)
	return nil
}

// 为 Logger 删除侧输出流
// func (l *Logger) Detach(name string) error {
// 	l.Lock()
// 	defer l.Unlock()
// 	return l.detach(name)
// }

// func (l *Logger) detach(name string) error {
// 	outputs := make([]*outputLogger, 0)
// 	for _, v := range l.outputs {
// 		if v.name == name {
// 			continue
// 		}
// 		outputs = append(outputs, v)
// 	}
// 	l.outputs = outputs
// 	return nil
// }

// 为 Logger 设置异步输出，目前只是简单将标识为设置为 false
func (l *Logger) async(data ...int) {
	l.Lock()
	defer l.Unlock()
	l.synchronous = false

	msgChanLen := 100
	if len(data) > 0 {
		msgChanLen = data[0]
	}

	l.msgChan = make(chan *message, msgChanLen)

	go func() {
		defer func() {
			// 让宕机的进程中的 goroutine 恢复过来
			// recover 仅在延迟函数 defer 中有效
			// 如果当前的 goroutine 陷入恐慌，调用 recover 可以捕获到 panic 的输入值，并且恢复正常的执行
			// 在程序关闭前做一些操作
			e := recover()
			if e != nil {
				fmt.Printf("%v", e)
				l.Flush()
			}
		}()
		l.startAsyncwrite()
	}()
}

func Async(data ...int) {
	Log.async(data...)
}

func (l *Logger) writeMsg(level LogLevel, msg string) (int, error) {
	funcName := "null"
	// Caller() 报告当前go程调用栈所执行的函数的文件和行号信息。
	// skip 表示上溯的栈帧数
	pc, file, line, ok := runtime.Caller(2)
	if !ok {
		file, line = "null", 0
	} else {
		funcName = runtime.FuncForPC(pc).Name()
	}
	_, fileName := path.Split(file)
	loggerMsg := &message{
		Timestamp:         time.Now().Unix(),
		TimestampFormat:   time.Now().Format("2006-01-02 15:04:05"),
		Millisecond:       time.Now().UnixNano() / 1e6,
		MillisecondFormat: time.Now().Format("2006-01-02 15:04:05.999"),
		Level:             level,
		LevelString:       levelStringMapping[level],
		Body:              msg,
		File:              fileName,
		Line:              line,
		Function:          funcName,
	}

	if l.synchronous {
		l.writeToOutputs(loggerMsg)
	} else {
		// 异步输出
		l.wg.Add(1)
		l.msgChan <- loggerMsg
	}
	return 0, nil
}

// 对 io.Write 接口的实现，默认输出到 Info 级别
func (l *Logger) Write(p []byte) (int, error) {
	switch globalLevel {
	case TRACE:
		l.Trace(string(p))
	case DEBUG:
		l.Debug(string(p))
	case INFO:
		l.Debug(string(p))
	case WARN:
		l.Warn(string(p))
	case ERROR:
		l.Debug(string(p))
	case FATAL:
		l.Fatal(string(p))
	}
	return 0, nil
}

// 将日志信息输出到每一个侧输出流中
func (l *Logger) writeToOutputs(msg *message) {
	for _, logger := range l.outputs {
		if logger.level <= msg.Level {
			if err := logger.logger.writeMsg(msg); err != nil {
				fmt.Fprintf(os.Stderr, "logger.%s: unable write loggerMessage to adapter:%v, error: %v\n", l.name, logger.logType, err)
			}
		}
	}
}

// 异步输出
func (l *Logger) startAsyncwrite() {
	for {
		select {
		case message := <-l.msgChan:
			l.writeToOutputs(message)
			l.wg.Done()
		case sign := <-l.signalChan:
			if sign == "flush" {
				l.flush()
				return
			}
		}
	}
}

// TODO 更高级的变量解析实现
func messageFormat(format string, msg *message) string {
	message := strings.Replace(format, "%timestamp%", strconv.FormatInt(msg.Timestamp, 10), 1)
	message = strings.Replace(message, "%timestamp_format%", msg.TimestampFormat, 1)
	message = strings.Replace(message, "%millisecond%", strconv.FormatInt(msg.Millisecond, 10), 1)
	message = strings.Replace(message, "%millisecond_format%", msg.MillisecondFormat, 1)
	message = strings.Replace(message, "%level%", strconv.Itoa(int(msg.Level)), 1)
	message = strings.Replace(message, "%level_string%", msg.LevelString, 1)
	message = strings.Replace(message, "%file%", msg.File, 1)
	message = strings.Replace(message, "%line%", strconv.Itoa(msg.Line), 1)
	message = strings.Replace(message, "%function%", msg.Function, 1)
	message = strings.Replace(message, "%body%", msg.Body, 1)
	message = strings.Replace(message, "%server_name%", viper.GetString("server.name"), 1)
	return message
}

func (l *Logger) Flush() {
	if !l.synchronous {
		l.signalChan <- "flush"
		l.wg.Wait()
		return
	}
	l.flush()
}

func (l *Logger) flush() {
	if !l.synchronous {
		for {
			if len(l.msgChan) > 0 {
				msg := <-l.msgChan
				l.writeToOutputs(msg)
				l.wg.Done()
				continue
			}
			break
		}
		for _, output := range l.outputs {
			output.logger.Flush()
		}
	}
}

func Flush() {
	Log.Flush()
}

func (l *Logger) Trace(msg string) {
	l.writeMsg(TRACE, msg)
}

func (l *Logger) Debug(msg string) {
	l.writeMsg(DEBUG, msg)
}

func (l *Logger) Info(msg string) {
	l.writeMsg(INFO, msg)
}

func (l *Logger) Warn(msg string) {
	l.writeMsg(WARN, msg)
}

func (l *Logger) Error(msg string) {
	l.writeMsg(ERROR, msg)
}

func (l *Logger) Fatal(msg string) {
	l.writeMsg(FATAL, msg)
	// Fatal 会导致程序停止，在异步情况下先把日志全部输出完在结束程序
	panic(msg)
}

func (l *Logger) Tracef(format string, a ...interface{}) {
	msg := fmt.Sprintf(format, a...)
	l.writeMsg(TRACE, msg)
}

func (l *Logger) Debugf(format string, a ...interface{}) {
	msg := fmt.Sprintf(format, a...)
	l.writeMsg(DEBUG, msg)
}

func (l *Logger) Infof(format string, a ...interface{}) {
	msg := fmt.Sprintf(format, a...)
	l.writeMsg(INFO, msg)
}

func (l *Logger) Warnf(format string, a ...interface{}) {
	msg := fmt.Sprintf(format, a...)
	l.writeMsg(WARN, msg)
}

func (l *Logger) Errorf(format string, a ...interface{}) {
	msg := fmt.Sprintf(format, a...)
	l.writeMsg(ERROR, msg)
}

func (l *Logger) Printf(format string, a ...interface{}) {
	msg := fmt.Sprintf(format, a...)
	l.writeMsg(globalLevel, msg)
}

func Trace(msg string) {
	Log.writeMsg(TRACE, msg)
}

func Debug(msg string) {
	Log.writeMsg(DEBUG, msg)
}

func Info(msg string) {
	Log.writeMsg(INFO, msg)
}

func Warn(msg string) {
	Log.writeMsg(WARN, msg)
}

func Error(msg string) {
	Log.writeMsg(ERROR, msg)
}

func Fatal(msg string) {
	Log.writeMsg(FATAL, msg)
	// Fatal 会导致程序停止，在异步情况下先把日志全部输出完在结束程序
	panic(msg)
}

func Tracef(format string, a ...interface{}) {
	msg := fmt.Sprintf(format, a...)
	Log.writeMsg(TRACE, msg)
}

func Debugf(format string, a ...interface{}) {
	msg := fmt.Sprintf(format, a...)
	Log.writeMsg(DEBUG, msg)
}

func Infof(format string, a ...interface{}) {
	msg := fmt.Sprintf(format, a...)
	Log.writeMsg(INFO, msg)
}

func Warnf(format string, a ...interface{}) {
	msg := fmt.Sprintf(format, a...)
	Log.writeMsg(WARN, msg)
}

func Errorf(format string, a ...interface{}) {
	msg := fmt.Sprintf(format, a...)
	Log.writeMsg(ERROR, msg)
}

func Fatalf(format string, a ...interface{}) {
	msg := fmt.Sprintf(format, a...)
	Log.writeMsg(FATAL, msg)
	panic(msg)
}
