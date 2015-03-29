// Copyright 2014 beego Author. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Usage:
//
// import "github.com/astaxie/beego/logs"
//
//	log := NewLogger(10000)
//	log.SetLogger("console", "")
//
//	> the first params stand for how many channel
//
// Use it like this:
//
//	log.Trace("trace")
//	log.Info("info")
//	log.Warn("warning")
//	log.Debug("debug")
//	log.Critical("critical")
//
//  more docs http://beego.me/docs/module/logs.md
package logs

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"path"
	"reflect"
	"runtime"
	"strings"
	"sync"

	"github.com/gogap/errors"
)

// RFC5424 log message levels.
const (
	LevelError = iota
	LevelWarn
	LevelInfo
	LevelDebug
)

// Legacy loglevel constants to ensure backwards compatibility.
//
// Deprecated: will be removed in 1.5.0.

type loggerType func() LoggerInterface

// LoggerInterface defines the behavior of a log provider.
type LoggerInterface interface {
	Init(config string) error
	WriteMsg(msg string, level int) error
	Destroy()
	Flush()
}

var adapters = make(map[string]loggerType)

// Register makes a log provide available by the provided name.
// If Register is called twice with the same name or if driver is nil,
// it panics.
func Register(name string, log loggerType) {
	if log == nil {
		panic("logs: Register provide is nil")
	}
	if _, dup := adapters[name]; dup {
		panic("logs: Register called twice for provider " + name)
	}
	adapters[name] = log
}

// Logger is default logger in beego application.
// it can contain several providers and log message into all providers.
type Logger struct {
	lock                sync.Mutex
	level               int
	enableFuncCallDepth bool
	loggerFuncCallDepth int
	msg                 chan *logMsg
	outputs             map[string]LoggerInterface
}

type logMsg struct {
	level int
	msg   string
}

// NewLogger returns a new Logger.
// channellen means the number of messages in chan.
// if the buffering chan is full, logger adapters write to file or other way.
func NewLogger(channellen int64) *Logger {
	bl := new(Logger)
	bl.level = LevelDebug
	bl.loggerFuncCallDepth = 2
	bl.EnableFuncCallDepth(true)
	bl.msg = make(chan *logMsg, channellen)
	bl.outputs = make(map[string]LoggerInterface)
	bl.SetLogger("console", "") // default output to console
	go bl.startLogger()
	return bl
}

func NewFileLogger(file string) *Logger {
	l := NewLogger(1024)
	path := strings.Split(file, "/")
	if len(path) > 1 {
		exec.Command("mkdir", path[0]).Run()
	}
	l.SetLogger("file", fmt.Sprintf(`{"filename":"%s","maxdays":7}`, file))
	l.EnableFuncCallDepth(true)
	l.SetLogFuncCallDepth(4)
	return l
}

// SetLogger provides a given logger adapter into Logger with config string.
// config need to be correct JSON as string: {"interval":360}.
func (bl *Logger) SetLogger(adaptername string, config string) error {
	bl.lock.Lock()
	defer bl.lock.Unlock()
	if log, ok := adapters[adaptername]; ok {
		lg := log()
		err := lg.Init(config)
		bl.outputs[adaptername] = lg
		if err != nil {
			fmt.Println("logs.Logger.SetLogger: " + err.Error())
			return err
		}
	} else {
		return fmt.Errorf("logs: unknown adaptername %q (forgotten Register?)", adaptername)
	}
	return nil
}

// remove a logger adapter in Logger.
func (bl *Logger) DelLogger(adaptername string) error {
	bl.lock.Lock()
	defer bl.lock.Unlock()
	if lg, ok := bl.outputs[adaptername]; ok {
		lg.Destroy()
		delete(bl.outputs, adaptername)
		return nil
	} else {
		return fmt.Errorf("logs: unknown adaptername %q (forgotten Register?)", adaptername)
	}
}

func (bl *Logger) writerMsg(loglevel int, msg string) error {
	if loglevel > bl.level {
		return nil
	}
	lm := new(logMsg)
	lm.level = loglevel
	if bl.enableFuncCallDepth {
		_, file, line, ok := runtime.Caller(bl.loggerFuncCallDepth)
		if ok {
			_, filename := path.Split(file)
			lm.msg = fmt.Sprintf("[%s:%d] %s", filename, line, msg)
		} else {
			lm.msg = msg
		}
	} else {
		lm.msg = msg
	}
	bl.msg <- lm
	return nil
}

// Set log message level.
//
// If message level (such as LevelDebug) is higher than logger level (such as LevelWarning),
// log providers will not even be sent the message.
func (bl *Logger) SetLevel(l int) {
	bl.level = l
}

// set log funcCallDepth
func (bl *Logger) SetLogFuncCallDepth(d int) {
	bl.loggerFuncCallDepth = d
}

// enable log funcCallDepth
func (bl *Logger) EnableFuncCallDepth(b bool) {
	bl.enableFuncCallDepth = b
}

// start logger chan reading.
// when chan is not empty, write logs.
func (bl *Logger) startLogger() {
	for {
		select {
		case bm := <-bl.msg:
			for _, l := range bl.outputs {
				err := l.WriteMsg(bm.msg, bm.level)
				if err != nil {
					fmt.Println("ERROR, unable to WriteMsg:", err)
				}
			}
		}
	}
}

// Log ERROR level message.
func (bl *Logger) Error(v ...interface{}) {
	bl.log("Error", LevelError, v)
}

// Log WARNING level message.
func (bl *Logger) Warn(v ...interface{}) {
	bl.log("Warn", LevelWarn, v)
}

// Log INFORMATIONAL level message.
func (bl *Logger) Info(v ...interface{}) {
	bl.log("Info", LevelInfo, v)
}

// Log DEBUG level message.
func (bl *Logger) Debug(v ...interface{}) {
	bl.log("Debug", LevelDebug, v)
}

func (bl *Logger) log(tp string, level int, v ...interface{}) {
	msg := fmt.Sprintf("["+tp+"] "+generateFmtStr(len(v)), v...)

	stack := handleError(rotate(v))
	if stack != "" {
		msg = msg + "\n" + stack
	}
	bl.writerMsg(level, msg)
}

//simple and strong, niu b !
func rotate(item interface{}) interface{} {
	if items, ok := item.([]interface{}); ok {
		for _, item = range items {
			if errcode, ok := rotate(item).(errors.ErrCode); ok {
				return errcode
			}
		}
	}
	return item
}

func (bl *Logger) Pretty(message string, v interface{}) {
	bl.pretty(message, v)
}
func (bl *Logger) pretty(message string, v interface{}) {
	b, _ := json.MarshalIndent(v, " ", "  ")
	if message == "" {
		message = reflect.TypeOf(v).String()
	}
	bl.writerMsg(LevelDebug, fmt.Sprintf("[Pretty]\n%s\n%s", message, string(b)))
}

// flush all chan data.
func (bl *Logger) Flush() {
	for _, l := range bl.outputs {
		l.Flush()
	}
}

// close logger, flush all chan data and destroy all adapters in Logger.
func (bl *Logger) Close() {
	for {
		if len(bl.msg) > 0 {
			bm := <-bl.msg
			for _, l := range bl.outputs {
				err := l.WriteMsg(bm.msg, bm.level)
				if err != nil {
					fmt.Println("ERROR, unable to WriteMsg (while closing logger):", err)
				}
			}
		} else {
			break
		}
	}
	for _, l := range bl.outputs {
		l.Flush()
		l.Destroy()
	}
}

func generateFmtStr(n int) string {
	return strings.Repeat("%v ", n)
}

func handleError(v interface{}) (msg string) {
	if err, ok := v.(errors.ErrCode); ok {
		msg = msg + err.StackTrace()
	}
	return
}
