package parser

import "strings"

var CommentParsers = map[string]CommentParser{
	".go":  GoCommentParser{},
	".js":  JsCommentParser{},
	".ts":  JsCommentParser{},
	".jsx": JsCommentParser{},
	".tsx": JsCommentParser{},
	// Add more as needed
}

type CommentParser interface {
	IsComment(line string) bool
}

type GoCommentParser struct{}

func (GoCommentParser) IsComment(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") || strings.HasPrefix(trimmed, "*") || strings.HasPrefix(trimmed, "*/")
}

type JsCommentParser struct{}

func (JsCommentParser) IsComment(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") || strings.HasPrefix(trimmed, "*") || strings.HasPrefix(trimmed, "*/")
}
