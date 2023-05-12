package logger

import (
	"errors"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/qiancijun/vermouth/config"
	"github.com/qiancijun/vermouth/utils"
)

const (
	FILE_ADAPTER_NAME     = "file"
	FILE_DATE_SLICE_NULL  = ""
	FILE_DATE_SLICE_HOUR  = "h"
	FILE_DATE_SLICE_DAY   = "d"
	FILE_DATE_SLICE_MONTH = "m"
	FILE_DATE_SLICE_YEAR  = "y"
	YEAR_FORMAT       = "2006"
	MONTH_FORMAT      = "200601"
	DAY_FORMAT        = "20060102"
	HOUR_FORMAT       = "2006010206"
)

type FileAdapter struct {
	writer *FileWriter
	cfg config.LogArgs
}

type FileWriter struct {
	sync.Mutex
	writer    *os.File
	startLine int64
	startTime int64
	fileName  string
}

var _ iLogger = (*FileAdapter)(nil)

var (
	fileDateSliceMapping = map[string]int{
		FILE_DATE_SLICE_YEAR:  0,
		FILE_DATE_SLICE_MONTH: 1,
		FILE_DATE_SLICE_DAY:   2,
		FILE_DATE_SLICE_HOUR:  3,
	}
)

func init() {
	Register(FILE_ADAPTER_NAME, NewFileAdapter)
}

func NewFileWriter(fn string) *FileWriter {
	return &FileWriter{
		fileName: fn,
	}
}

func NewFileAdapter() iLogger {
	return &FileAdapter{}
}

func (f *FileAdapter) Name() string {
	return FILE_ADAPTER_NAME
}

func (f *FileAdapter) Init(args config.LogArgs) error {
	if args.Type != FILE_ADAPTER_NAME {
		return errors.New("logger console adapter init error, config must FileConfig, please check config file")
	}
	f.cfg = args

	if f.cfg.FileName == "" {
		return errors.New("file logger has a null path file, please check config")
	}

	if f.cfg.Format == "" {
		f.cfg.Format = defaultLoggerMessageFormat
	}

	_, ok := fileDateSliceMapping[f.cfg.DateSlice]
	if f.cfg.DateSlice != "" && !ok {
		return errors.New("date slice must be one of 'y', 'd', 'm', 'h'")
	}

	fw := NewFileWriter(f.cfg.FileName)
	fw.initFile()
	f.writer = fw
	return nil
}

func (f *FileAdapter) writeMsg(msg *message) error {
	accessChan := make(chan error)

	go func() {
		accessFileWriter := f.writer
		err := accessFileWriter.writeByConfig(f.cfg, msg)
		if err != nil {
			accessChan <- err
			return
		}
		accessChan <- nil
	}()

	var accessError error
	if f.cfg.FileName != "" {
		accessError = <-accessChan
	}

	if accessError != nil {
		return accessError
	}

	return nil
}

func (f *FileAdapter) Flush() {
	f.writer.writer.Close()
}

// ========== FileWriter ==========
func (fw *FileWriter) initFile() error {
	// 检查日志文件是否存在，不存在创建一个日志文件
	ok, _ := utils.PathExists(fw.fileName)
	if !ok {
		// 日志文件不存在，创建一个日志文件
		err := utils.CreateFile(fw.fileName)
		if err != nil {
			println(err.Error())
			return err
		}
	}

	// 填补 FileWriter 剩余的信息
	fw.startTime = time.Now().Unix()

	curLines, err := utils.GetFileLines(fw.fileName)
	if err != nil {
		return err
	}
	fw.startLine = curLines

	// 把文件标志符交给 FileWriter
	file, err := fw.getFileObject(fw.fileName)
	if err != nil {
		return err
	}
	fw.writer = file
	return nil
}

func (fw *FileWriter) getFileObject(fileName string) (*os.File, error) {
	file, err := os.OpenFile(fileName, os.O_RDWR|os.O_APPEND, 0766)
	return file, err
}

// 获取文件大小，单位：byte
func (fw *FileWriter) getFileSize(fileName string) (int64, error) {
	fileInfo, err := os.Stat(fileName)
	if err != nil {
		return 0, err
	}
	return fileInfo.Size() / 1024, nil
}

func (fw *FileWriter) writeByConfig(cfg config.LogArgs, msg *message) error {
	fw.Lock()
	defer fw.Unlock()

	// 检查是否需要进行归档切分
	if cfg.DateSlice != "" {
		if err := fw.sliceByDate(cfg.DateSlice); err != nil {
			return err
		}
	}

	// 检查是否超出最大行数限制
	if cfg.MaxLine != 0 {
		if err := fw.sliceByLine(cfg.MaxLine); err != nil {
			return err
		}
	}

	// 最大是否超出最大字节数
	if cfg.MaxSize != 0 {
		if err := fw.sliceBySize(cfg.MaxSize); err != nil {
			return err
		}
	}

	message := messageFormat(cfg.Format, msg) + "\r\n"
	_, err := fw.writer.Write([]byte(message))
	if cfg.MaxLine != 0 {
		fw.startLine += int64(strings.Count(message, "\n"))
	}

	return err
}

// 根据时间切分日志
func (fw *FileWriter) sliceByDate(dateSlice string) error {
	fileName := fw.fileName
	suffix := path.Ext(fileName)
	startTime, nowTime := time.Unix(fw.startTime, 0), time.Now()

	oldFileName := ""
	isHaveSlice := false

	if dateSlice == FILE_DATE_SLICE_YEAR &&
		startTime.Year() != nowTime.Year() {
		isHaveSlice = true
		oldFileName = strings.Replace(fileName, suffix, "", 1) +
			"_" +
			startTime.Format(YEAR_FORMAT) +
			suffix
	}
	if dateSlice == FILE_DATE_SLICE_MONTH &&
		startTime.Format(MONTH_FORMAT) != nowTime.Format(MONTH_FORMAT) {
		isHaveSlice = true
		oldFileName = strings.Replace(fileName, suffix, "", 1) +
			"_" +
			startTime.Format(MONTH_FORMAT) +
			suffix
	}
	if dateSlice == FILE_DATE_SLICE_DAY &&
		startTime.Format(DAY_FORMAT) != nowTime.Format(DAY_FORMAT) {
		isHaveSlice = true
		oldFileName = strings.Replace(fileName, suffix, "", 1) +
			"_" +
			startTime.Format(DAY_FORMAT) +
			suffix
	}
	if dateSlice == FILE_DATE_SLICE_HOUR &&
		startTime.Format(HOUR_FORMAT) != nowTime.Format(HOUR_FORMAT) {
		isHaveSlice = true
		oldFileName = strings.Replace(fileName, suffix, "", 1) +
			"_" +
			startTime.Format(HOUR_FORMAT) +
			suffix
	}

	if isHaveSlice {
		fw.writer.Close()
		if err := os.Rename(fw.fileName, oldFileName); err != nil {
			return err
		}
		if err := fw.initFile(); err != nil {
			return err
		}
	}

	return nil
}

// 根据行数切分日志
func (fw *FileWriter) sliceByLine(maxLine int64) error {
	fileName := fw.fileName
	suffix := path.Ext(fileName)
	startLine := fw.startLine

	if startLine >= maxLine {
		fw.writer.Close()
		timeFlag := time.Now().Format("2006-01-02-03.04.05.9999")
		oldFileName := strings.Replace(fileName, suffix, "", 1) +
			"." +
			timeFlag +
			suffix
		println(oldFileName)
		if err := os.Rename(fileName, oldFileName); err != nil {
			return err
		}
		if err := fw.initFile(); err != nil {
			return err
		}
	}

	return nil
}

// 根据字节数切分日志
func (fw *FileWriter) sliceBySize(maxSize int64) error {
	fileName := fw.fileName
	suffix := path.Ext(fileName)
	curSize, err := fw.getFileSize(fileName)
	if err != nil {
		return err
	}

	if curSize >= maxSize {
		fw.writer.Close()
		timeFlag := time.Now().Format("2006-01-02-03.04.05")
		oldFileName := strings.Replace(fileName, suffix, "", 1) +
			"." +
			timeFlag +
			suffix
		if err := os.Rename(fileName, oldFileName); err != nil {
			return err
		}
		if err := fw.initFile(); err != nil {
			return err
		}
	}
	return nil
}
