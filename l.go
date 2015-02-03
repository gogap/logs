package logs

var logger *Logger

func init() {
	logger = NewLogger(1024)
	logger.SetLogFuncCallDepth(4)
}

/*
// set file logger with json config.
// jsonconfig like:
//	{
//	"filename":"logs/sample.log",
//	"maxlines":10000,
//	"maxsize":1<<30,
//	"daily":true,
//	"maxdays":15,
//	"rotate":true
//	}
*/
func SetFileLogger(file string) {
	logger = NewFileLogger(file)
}
func Error(v ...interface{}) {
	logger.Error(v...)
}
func Warn(v ...interface{}) {
	logger.Warn(v...)
}
func Info(v ...interface{}) {
	logger.Info(v...)
}
func Debug(v ...interface{}) {
	logger.Debug(v...)
}
func Pretty(message string, v interface{}) {
	logger.Pretty(message, v)
}
func SetLoggerLevel(l int) {
	logger.SetLevel(l)
}
