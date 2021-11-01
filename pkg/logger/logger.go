package logger

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"path"
	"strings"

	"github.com/sirupsen/logrus"
)

const (
	logDirectory = "/kubeshare/log/"
)

type KubeShareFormatter struct {
}

func (ksf *KubeShareFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	var b *bytes.Buffer
	if entry.Buffer != nil {
		b = entry.Buffer
	} else {
		b = &bytes.Buffer{}
	}
	timestamp := entry.Time.Format("2006-01-02 15:04:05")
	var newLog string
	fileName := path.Base(entry.Caller.File)
	level := entry.Level.String()
	if len(level) > 4 {
		level = level[:4]
	}
	level = strings.ToUpper(level)
	newLog = fmt.Sprintf("%s %s: %s:%d %s\n", timestamp, level, fileName, entry.Caller.Line, entry.Message)
	b.WriteString(newLog)
	return b.Bytes(), nil
}
func New(level int64, filename string) *logrus.Logger {
	level += 2
	if level > 5 || level < 2 {
		level = 4 // Info
	}
	logger := logrus.New()
	logger.SetLevel(logrus.AllLevels[level])
	logger.SetReportCaller(true)
	logger.SetFormatter(&KubeShareFormatter{})
	os.MkdirAll(logDirectory, os.ModePerm)
	filePath := fmt.Sprintf(logDirectory + filename)
	file, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		log.Fatal(err)
	}
	logger.SetOutput(file)
	return logger
}
