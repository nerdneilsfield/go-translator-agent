package document

import (
	"encoding/xml"
)

// DOCX XML Namespaces
const (
	WordprocessingMLNamespace = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"
	RelationshipsNamespace    = "http://schemas.openxmlformats.org/package/2006/relationships"
)

// WordDocument represents the main document.xml structure
type WordDocument struct {
	XMLName xml.Name `xml:"document"`
	Body    Body     `xml:"body"`
}

// Body represents the document body
type Body struct {
	Paragraphs []Paragraph `xml:"p"`
	Tables     []Table     `xml:"tbl"`
}

// Paragraph represents a paragraph element
type Paragraph struct {
	XMLName    xml.Name        `xml:"p"`
	Properties *ParagraphProps `xml:"pPr"`
	Runs       []Run           `xml:"r"`
}

// ParagraphProps represents paragraph properties
type ParagraphProps struct {
	Style   *ParagraphStyle   `xml:"pStyle"`
	Spacing *ParagraphSpacing `xml:"spacing"`
	Align   *ParagraphAlign   `xml:"jc"`
}

// ParagraphStyle represents paragraph style
type ParagraphStyle struct {
	Val string `xml:"val,attr"`
}

// ParagraphSpacing represents paragraph spacing
type ParagraphSpacing struct {
	After  string `xml:"after,attr,omitempty"`
	Before string `xml:"before,attr,omitempty"`
	Line   string `xml:"line,attr,omitempty"`
}

// ParagraphAlign represents paragraph alignment
type ParagraphAlign struct {
	Val string `xml:"val,attr"`
}

// Run represents a text run
type Run struct {
	XMLName    xml.Name     `xml:"r"`
	Properties *RunProps    `xml:"rPr"`
	Text       *Text        `xml:"t"`
	Tab        *Tab         `xml:"tab"`
	Break      *Break       `xml:"br"`
	Drawing    *Drawing     `xml:"drawing"`
}

// RunProps represents run properties
type RunProps struct {
	Bold          *Bold          `xml:"b"`
	Italic        *Italic        `xml:"i"`
	Underline     *Underline     `xml:"u"`
	Strike        *Strike        `xml:"strike"`
	Color         *Color         `xml:"color"`
	Size          *FontSize      `xml:"sz"`
	Font          *RunFont       `xml:"rFonts"`
	Highlight     *Highlight     `xml:"highlight"`
}

// Text represents actual text content
type Text struct {
	XMLName xml.Name `xml:"t"`
	Space   string   `xml:"http://www.w3.org/XML/1998/namespace space,attr,omitempty"`
	Text    string   `xml:",chardata"`
}

// Tab represents a tab character
type Tab struct {
	XMLName xml.Name `xml:"tab"`
}

// Break represents a line break
type Break struct {
	XMLName xml.Name `xml:"br"`
	Type    string   `xml:"type,attr,omitempty"`
}

// Bold represents bold formatting
type Bold struct {
	Val string `xml:"val,attr,omitempty"`
}

// Italic represents italic formatting
type Italic struct {
	Val string `xml:"val,attr,omitempty"`
}

// Underline represents underline formatting
type Underline struct {
	Val string `xml:"val,attr,omitempty"`
}

// Strike represents strikethrough formatting
type Strike struct {
	Val string `xml:"val,attr,omitempty"`
}

// Color represents text color
type Color struct {
	Val string `xml:"val,attr"`
}

// FontSize represents font size
type FontSize struct {
	Val string `xml:"val,attr"`
}

// RunFont represents font settings
type RunFont struct {
	ASCII    string `xml:"ascii,attr,omitempty"`
	HAnsi    string `xml:"hAnsi,attr,omitempty"`
	EastAsia string `xml:"eastAsia,attr,omitempty"`
}

// Highlight represents text highlighting
type Highlight struct {
	Val string `xml:"val,attr"`
}

// Table represents a table element
type Table struct {
	XMLName    xml.Name      `xml:"tbl"`
	Properties *TableProps   `xml:"tblPr"`
	Grid       *TableGrid    `xml:"tblGrid"`
	Rows       []TableRow    `xml:"tr"`
}

// TableProps represents table properties
type TableProps struct {
	Style   *TableStyle   `xml:"tblStyle"`
	Width   *TableWidth   `xml:"tblW"`
	Borders *TableBorders `xml:"tblBorders"`
}

// TableStyle represents table style
type TableStyle struct {
	Val string `xml:"val,attr"`
}

// TableWidth represents table width
type TableWidth struct {
	Type string `xml:"type,attr"`
	W    string `xml:"w,attr"`
}

// TableBorders represents table borders
type TableBorders struct {
	Top    *Border `xml:"top"`
	Left   *Border `xml:"left"`
	Bottom *Border `xml:"bottom"`
	Right  *Border `xml:"right"`
}

// Border represents a border
type Border struct {
	Val   string `xml:"val,attr"`
	Sz    string `xml:"sz,attr,omitempty"`
	Space string `xml:"space,attr,omitempty"`
	Color string `xml:"color,attr,omitempty"`
}

// TableGrid represents table grid
type TableGrid struct {
	GridCols []GridCol `xml:"gridCol"`
}

// GridCol represents a grid column
type GridCol struct {
	W string `xml:"w,attr"`
}

// TableRow represents a table row
type TableRow struct {
	XMLName    xml.Name        `xml:"tr"`
	Properties *TableRowProps  `xml:"trPr"`
	Cells      []TableCell     `xml:"tc"`
}

// TableRowProps represents table row properties
type TableRowProps struct {
	Height *RowHeight `xml:"trHeight"`
}

// RowHeight represents row height
type RowHeight struct {
	Val string `xml:"val,attr"`
}

// TableCell represents a table cell
type TableCell struct {
	XMLName    xml.Name         `xml:"tc"`
	Properties *TableCellProps  `xml:"tcPr"`
	Paragraphs []Paragraph      `xml:"p"`
}

// TableCellProps represents table cell properties
type TableCellProps struct {
	Width    *TableCellWidth    `xml:"tcW"`
	Borders  *TableCellBorders  `xml:"tcBorders"`
	Shading  *TableCellShading  `xml:"shd"`
	VMerge   *VerticalMerge     `xml:"vMerge"`
	GridSpan *GridSpan          `xml:"gridSpan"`
}

// TableCellWidth represents cell width
type TableCellWidth struct {
	Type string `xml:"type,attr"`
	W    string `xml:"w,attr"`
}

// TableCellBorders represents cell borders
type TableCellBorders struct {
	Top    *Border `xml:"top"`
	Left   *Border `xml:"left"`
	Bottom *Border `xml:"bottom"`
	Right  *Border `xml:"right"`
}

// TableCellShading represents cell shading
type TableCellShading struct {
	Val   string `xml:"val,attr"`
	Color string `xml:"color,attr,omitempty"`
	Fill  string `xml:"fill,attr,omitempty"`
}

// VerticalMerge represents vertical merge
type VerticalMerge struct {
	Val string `xml:"val,attr,omitempty"`
}

// GridSpan represents grid span
type GridSpan struct {
	Val string `xml:"val,attr"`
}

// Drawing represents a drawing/image element
type Drawing struct {
	XMLName xml.Name `xml:"drawing"`
	// Simplified - actual structure is complex
	// We'll preserve the entire element during translation
}

// Hyperlink represents a hyperlink
type Hyperlink struct {
	XMLName xml.Name `xml:"hyperlink"`
	ID      string   `xml:"http://schemas.openxmlformats.org/officeDocument/2006/relationships id,attr"`
	Runs    []Run    `xml:"r"`
}

// BookmarkStart represents bookmark start
type BookmarkStart struct {
	XMLName xml.Name `xml:"bookmarkStart"`
	ID      string   `xml:"id,attr"`
	Name    string   `xml:"name,attr"`
}

// BookmarkEnd represents bookmark end
type BookmarkEnd struct {
	XMLName xml.Name `xml:"bookmarkEnd"`
	ID      string   `xml:"id,attr"`
}

// ContentTypes represents [Content_Types].xml
type ContentTypes struct {
	XMLName   xml.Name   `xml:"Types"`
	Namespace string     `xml:"xmlns,attr"`
	Defaults  []Default  `xml:"Default"`
	Overrides []Override `xml:"Override"`
}

// Default represents a default content type
type Default struct {
	Extension   string `xml:"Extension,attr"`
	ContentType string `xml:"ContentType,attr"`
}

// Override represents an override content type
type Override struct {
	PartName    string `xml:"PartName,attr"`
	ContentType string `xml:"ContentType,attr"`
}

// Relationships represents relationships
type Relationships struct {
	XMLName       xml.Name       `xml:"Relationships"`
	Namespace     string         `xml:"xmlns,attr"`
	Relationships []Relationship `xml:"Relationship"`
}

// Relationship represents a relationship
type Relationship struct {
	ID     string `xml:"Id,attr"`
	Type   string `xml:"Type,attr"`
	Target string `xml:"Target,attr"`
}

// DocxNamespaces returns common DOCX namespaces for XML parsing
func DocxNamespaces() map[string]string {
	return map[string]string{
		"w":   WordprocessingMLNamespace,
		"r":   RelationshipsNamespace,
		"a":   "http://schemas.openxmlformats.org/drawingml/2006/main",
		"wp":  "http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing",
		"w14": "http://schemas.microsoft.com/office/word/2010/wordml",
		"w15": "http://schemas.microsoft.com/office/word/2012/wordml",
	}
}