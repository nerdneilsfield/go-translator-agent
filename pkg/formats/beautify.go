package formats

import (
	"fmt"
	"go.uber.org/zap"
	"os/exec"
	"path/filepath"
	"strings"
)

// FormatFile 根据文件类型使用相应的外部工具进行格式化
func FormatFile(filePath string, logger *zap.Logger) error {
	ext := strings.ToLower(filepath.Ext(filePath))

	switch ext {
	case ".md", ".markdown":
		// if !checkCommand("prettier") {
		// 	return fmt.Errorf("prettier 未安装，请使用以下命令安装：npm install -g prettier")
		// }
		return FormatMarkdown(filePath, logger)
	case ".tex":
		if !checkCommand("latexindent") {
			return fmt.Errorf("latexindent 未安装，请安装 texlive 或相关 LaTeX 发行版")
		}
		return formatLatex(filePath, logger)
	case ".html", ".htm", ".css", ".js", ".xhtml":
		if !checkCommand("prettier") {
			return fmt.Errorf("prettier 未安装，请使用以下命令安装：npm install -g prettier")
		}
		return formatWithPrettier(filePath, logger)
	case ".java":
		if !checkCommand("google-java-format") {
			return fmt.Errorf("google-java-format 未安装，请参考 https://github.com/google/google-java-format")
		}
		return formatJava(filePath, logger)
	case ".epub":
		return nil
	case ".txt", ".text":
		return formatText(filePath, logger)
	default:
		return fmt.Errorf("不支持的文件类型: %s", ext)
	}
}

func checkCommand(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}

func formatLatex(filePath string, logger *zap.Logger) error {
	cmd := exec.Command("latexindent", "-w", filePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		logger.Error("latexindent 执行失败", zap.Error(err), zap.String("output", string(output)))
		return fmt.Errorf("latexindent 执行失败: %v\n输出: %s", err, string(output))
	}
	logger.Debug("latexindent 执行成功", zap.String("file", filePath))
	return nil
}

func formatText(filePath string, logger *zap.Logger) error {
	tf := NewTextFormattingProcessor()
	return tf.FormatFile(filePath, filePath)
}

func formatWithPrettier(filePath string, logger *zap.Logger) error {
	ext := strings.ToLower(filepath.Ext(filePath))
	args := []string{"--write"}

	// 根据文件类型添加特定配置
	switch ext {
	case ".md", ".markdown":
		args = append(args, "--parser", "markdown")
	case ".js":
		args = append(args, "--parser", "babel")
	case ".html", ".htm":
		args = append(args, "--parser", "html")
	case ".css":
		args = append(args, "--parser", "css")
	}

	args = append(args, filePath)
	cmd := exec.Command("prettier", args...)

	// 打印完整命令行
	cmdStr := fmt.Sprintf("prettier %s", strings.Join(args, " "))
	logger.Debug("执行格式化命令",
		zap.String("command", cmdStr),
		zap.String("file", filePath))

	output, err := cmd.CombinedOutput()
	if err != nil {
		logger.Error("prettier 执行失败", zap.Error(err), zap.String("output", string(output)))
		return fmt.Errorf("prettier 执行失败: %v\n输出: %s", err, string(output))
	}
	// fmt.Println()
	logger.Debug("prettier 执行成功", zap.String("file", filePath))
	return nil
}

func formatJava(filePath string, logger *zap.Logger) error {
	cmd := exec.Command("google-java-format", "-i", filePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		logger.Error("google-java-format 执行失败", zap.Error(err), zap.String("output", string(output)))
		return fmt.Errorf("google-java-format 执行失败: %v\n输出: %s", err, string(output))
	}
	logger.Debug("google-java-format 执行成功", zap.String("file", filePath))
	return nil
}
