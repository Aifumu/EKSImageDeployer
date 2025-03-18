package logger

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var (
	InfoLogger  *log.Logger
	ErrorLogger *log.Logger
	logFile     *os.File
)

// ANSI颜色代码
const (
	Reset      = "\033[0m"
	Red        = "\033[31m"
	Green      = "\033[32m"
	Yellow     = "\033[33m"
	Blue       = "\033[34m"
	Purple     = "\033[35m"
	Cyan       = "\033[36m"
	White      = "\033[37m"
	BoldWhite  = "\033[1;37m"
	BoldGreen  = "\033[1;32m"
	BoldYellow = "\033[1;33m"
)

// InitLogger 初始化日志记录器
func InitLogger() error {
	// 创建logs目录
	if err := os.MkdirAll("logs", 0755); err != nil {
		return fmt.Errorf("创建日志目录失败: %v", err)
	}

	// 生成日志文件名（精确到秒）
	logFileName := filepath.Join("logs", fmt.Sprintf("deploy_%s.log", time.Now().Format("2006-01-02_15-04-05")))
	
	// 打开日志文件（追加模式）
	file, err := os.OpenFile(logFileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("打开日志文件失败: %v", err)
	}

	logFile = file
	
	// 设置日志格式
	InfoLogger = log.New(file, "INFO: ", log.Ldate|log.Ltime)
	ErrorLogger = log.New(file, "ERROR: ", log.Ldate|log.Ltime)

	return nil
}

// CloseLogger 关闭日志文件
func CloseLogger() {
	if logFile != nil {
		logFile.Close()
	}
}

// Info 记录信息日志，同时输出到控制台
func Info(format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)
	fmt.Println(msg)
	if InfoLogger != nil {
		// 移除控制台颜色代码后写入日志文件
		cleanMsg := removeColorCodes(msg)
		InfoLogger.Println(cleanMsg)
	}
}

// Error 记录错误日志，同时输出到控制台
func Error(format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)
	fmt.Println(Red + msg + Reset)
	if ErrorLogger != nil {
		// 移除控制台颜色代码后写入日志文件
		cleanMsg := removeColorCodes(msg)
		ErrorLogger.Println(cleanMsg)
	}
}

// Success 输出成功信息
func Success(format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)
	fmt.Println(Green + msg + Reset)
	if InfoLogger != nil {
		// 移除控制台颜色代码后写入日志文件
		cleanMsg := removeColorCodes(msg)
		InfoLogger.Println(cleanMsg)
	}
}

// FormatJSON 格式化JSON并添加颜色
func FormatJSON(data interface{}) string {
	bytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Sprintf("JSON转换错误: %v", err)
	}

	jsonStr := string(bytes)
	lines := strings.Split(jsonStr, "\n")
	coloredLines := make([]string, len(lines))

	for i, line := range lines {
		// 处理不同层级的缩进和颜色
		trimmedLine := strings.TrimSpace(line)
		if strings.HasSuffix(trimmedLine, "{") || strings.HasSuffix(trimmedLine, "[") {
			// 对象或数组开始
			coloredLines[i] = BoldWhite + line + Reset
		} else if strings.HasSuffix(trimmedLine, "}") || strings.HasSuffix(trimmedLine, "]") {
			// 对象或数组结束
			coloredLines[i] = BoldWhite + line + Reset
		} else if strings.Contains(line, ":") {
			// 键值对
			parts := strings.SplitN(line, ":", 2)
			key := parts[0]
			value := parts[1]
			
			if strings.Contains(value, "\"") {
				// 字符串值
				coloredLines[i] = Cyan + key + BoldWhite + ":" + Green + value + Reset
			} else {
				// 数字或其他值
				coloredLines[i] = Cyan + key + BoldWhite + ":" + Yellow + value + Reset
			}
		} else {
			coloredLines[i] = line
		}
	}

	return strings.Join(coloredLines, "\n")
}

// removeColorCodes 移除ANSI颜色代码
func removeColorCodes(s string) string {
	colorCodes := []string{
		Reset, Red, Green, Yellow, Blue, Purple, Cyan, White,
		BoldWhite, BoldGreen, BoldYellow,
	}
	
	result := s
	for _, code := range colorCodes {
		result = strings.ReplaceAll(result, code, "")
	}
	return result
} 