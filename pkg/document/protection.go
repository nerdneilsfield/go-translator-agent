package document

// PatternProtector 模式保护接口，避免直接依赖translation包
type PatternProtector interface {
	ProtectPattern(text string, pattern string) string
	// Restore 恢复所有保护的占位符
	Restore(text string) string
}

// ContentProtector 内容保护器接口
type ContentProtector interface {
	// ProtectContent 保护指定格式的内容
	ProtectContent(text string, patternProtector PatternProtector) string
	// RestoreContent 恢复保护的内容
	RestoreContent(text string, patternProtector PatternProtector) string
	// GetProtectedPatterns 获取保护的模式列表（用于调试）
	GetProtectedPatterns() []string
	// GetFormatName 获取格式名称
	GetFormatName() string
}

// BaseProtector 基础保护器，提供通用保护功能
type BaseProtector struct {
	formatName string
}

// NewBaseProtector 创建基础保护器
func NewBaseProtector(formatName string) *BaseProtector {
	return &BaseProtector{formatName: formatName}
}

// GetFormatName 获取格式名称
func (bp *BaseProtector) GetFormatName() string {
	return bp.formatName
}

// ProtectCommonContent 保护通用内容（所有格式都需要的）
func (bp *BaseProtector) ProtectCommonContent(text string, pp PatternProtector) string {
	// URL保护
	text = pp.ProtectPattern(text, `(?i)(https?|ftp|file)://[^\s\)]+`)
	text = pp.ProtectPattern(text, `(?i)www\.[^\s\)]+`)
	
	// 邮箱地址
	text = pp.ProtectPattern(text, `[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`)
	
	// 文件路径
	text = pp.ProtectPattern(text, `(?:^|[\s(])/(?:[^/\s]+/)*[^/\s]+(?:\.[a-zA-Z0-9]+)?`)
	text = pp.ProtectPattern(text, `[A-Za-z]:\\(?:[^\\/:*?"<>|\r\n]+\\)*[^\\/:*?"<>|\r\n]+`)
	
	// IP地址
	text = pp.ProtectPattern(text, `\b(?:[0-9]{1,3}\.){3}[0-9]{1,3}\b`)
	
	// 技术版本号
	text = pp.ProtectPattern(text, `\bv?\d+\.\d+(?:\.\d+)?(?:-[a-zA-Z0-9]+)?(?:\+[a-zA-Z0-9]+)?\b`)
	
	return text
}

// GetCommonPatterns 获取通用保护模式
func (bp *BaseProtector) GetCommonPatterns() []string {
	return []string{
		"URLs", "Email addresses", "File paths", "IP addresses", "Version numbers",
	}
}