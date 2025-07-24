package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	ext_ast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"
	"golang.org/x/net/proxy"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/docs/v1"
	"google.golang.org/api/option"
)

// --- 可配置的样式 ---
const (
	// --- 主题颜色 ---
	colorPrimary      = "#0d6efd" // 科技蓝 (主色)
	colorPrimaryLight = "#e7f1ff" // 浅蓝 (背景)
	colorText         = "#333333" // 正文文本颜色
	colorHeaderText   = "#ffffff" // 白色 (用于深色背景标题)
	colorMuted        = "#6c757d" // 辅助/静音颜色 (用于标签)
	colorBorder       = "#dee2e6" // 边框颜色

	// --- 基础与布局 ---
	styleBody = `padding: 16px 20px; font-family: -apple-system, BlinkMacSystemFont, 'Helvetica Neue', 'PingFang SC', 'Microsoft YaHei', sans-serif; letter-spacing: 0.544px; font-size: 16px; line-height: 1.8; color: ` + colorText + `;`

	// --- 标题样式 ---
	styleH1 = `margin: 40px 0 25px; padding: 15px; font-size: 24px; font-weight: bold; line-height: 1.4; text-align: center; color: ` + colorHeaderText + `; background: linear-gradient(135deg, #0d6efd, #053b84); border-radius: 8px;`
	styleH2 = `margin: 35px 0 20px; padding-bottom: 8px; font-size: 20px; font-weight: bold; line-height: 1.4; color: ` + colorPrimary + `; border-bottom: 3px solid ` + colorPrimaryLight + `;`
	styleH3 = `margin: 30px 0 15px; padding-left: 12px; font-size: 18px; font-weight: bold; line-height: 1.4; color: #1e3a8a; border-left: 4px solid ` + colorPrimary + `;`

	// --- 内容元素 ---
	styleParagraph  = `margin-top: 1.2em; margin-bottom: 1.2em;`
	styleBlockquote = `padding: 15px 20px; margin: 25px 0; background-color: ` + colorPrimaryLight + `; border-left: 4px solid ` + colorPrimary + `; color: #053b84; font-size: 15px;`
	styleCodeBlock  = `display: block; overflow-x: auto; padding: 1.2em; background: #282c34; color: #abb2bf; margin: 25px 0; border-radius: 8px; font-family: 'SFMono-Regular', Consolas, 'Liberation Mono', Menlo, Courier, monospace; font-size: 14px; line-height: 1.5;`
	styleImage      = `max-width: 100%; height: auto; display: block; margin: 25px auto; border-radius: 8px; box-shadow: 0 8px 20px rgba(0,0,0,0.12);`

	// --- 列表 ---
	styleUnorderedList = `margin: 1.2em 0; padding-left: 25px; list-style-type: disc;`
	styleOrderedList   = `margin: 1.2em 0; padding-left: 25px;`
	styleListItem      = `margin-bottom: 0.8em;`

	// --- 表格卡片化样式 ---
	styleTableWrapper = `margin: 30px 0;`
	// 卡片容器 (代表一行数据)，包含所有视觉样式和动画
	styleDataCard = `margin-bottom: 16px; padding: 16px; border-radius: 8px; background-color: #fff; border: 1px solid ` + colorBorder + `; box-shadow: 0 4px 15px rgba(0,0,0,0.06); overflow: hidden; animation: fadeInUp 0.5s ease-out forwards; opacity: 0; transform: translateY(20px);`

	// 卡片内的每一行 "标签: 值"
	styleDataRow = `font-size: 15px; color: ` + colorText + `; margin: 0 0 10px 0; padding: 0; line-height: 1.6;`
	// 最后一个 P 标签移除下外边距
	styleDataRowLast = styleDataRow + ` margin-bottom: 0;`

	// 标签部分的样式 (例如 "职位: ")
	styleDataLabel = `font-weight: 600; color: ` + colorText + `; margin-right: 8px;`
)

// CSS Keyframes 动画定义 (保持不变)
const cssKeyframes = `
<style>
@keyframes fadeInUp {
  from {
    opacity: 0;
    transform: translateY(20px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}
</style>
`

// --- 样式配置结束 ---

func getClient(ctx context.Context, config *oauth2.Config) *http.Client {
	tokFile := "token.json"
	tok, err := tokenFromFile(tokFile)
	if err != nil {
		tok = getTokenFromWeb(ctx, config)
		saveToken(tokFile, tok)
	}
	return config.Client(ctx, tok)
}

func getTokenFromWeb(ctx context.Context, config *oauth2.Config) *oauth2.Token {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("请在浏览器中打开以下链接进行授权: \n%v\n", authURL)
	fmt.Print("输入授权码: ")

	var authCode string
	if _, err := fmt.Scan(&authCode); err != nil {
		log.Fatalf("无法读取授权码: %v", err)
	}

	tok, err := config.Exchange(ctx, authCode)
	if err != nil {
		log.Fatalf("无法从授权码换取 token: %v", err)
	}
	return tok
}

func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tok := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(tok)
	return tok, err
}

func saveToken(path string, token *oauth2.Token) {
	fmt.Printf("保存凭证文件到: %s\n", path)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Fatalf("无法缓存 oauth token: %v", err)
	}
	defer f.Close()
	json.NewEncoder(f).Encode(token)
}

// processDocument 是核心处理函数，增加了忽略标题、参考文献以及处理表格的功能
func processDocument(srv *docs.Service, docId string) (string, error) {
	doc, err := srv.Documents.Get(docId).Do()
	if err != nil {
		return "", fmt.Errorf("无法获取文档: %v", err)
	}

	var markdownBuilder strings.Builder

	// 定义需要忽略的章节标题
	stopHeadings := map[string]bool{
		"参考文献":       true,
		"引用的文献":      true,
		"References": true, // 也可加入英文
	}

	// 遍历文档内容元素
	for _, content := range doc.Body.Content {
		// --- 1. 处理段落 (Paragraph) ---
		if content.Paragraph != nil {
			para := content.Paragraph

			if para.ParagraphStyle.NamedStyleType == "TITLE" {
				fmt.Println("已忽略文档标题。")
				continue
			}

			var paraTextBuilder strings.Builder
			for _, elem := range para.Elements {
				if elem.TextRun != nil {
					paraTextBuilder.WriteString(elem.TextRun.Content)
				}
			}
			paraText := strings.TrimSpace(paraTextBuilder.String())

			if stopHeadings[paraText] {
				fmt.Printf("\n检测到章节 “%s”，已停止后续内容转换。\n", paraText)
				break
			}

			isHeading := false
			switch para.ParagraphStyle.NamedStyleType {
			case "HEADING_1":
				markdownBuilder.WriteString("# ")
				isHeading = true
			case "HEADING_2":
				markdownBuilder.WriteString("## ")
				isHeading = true
			case "HEADING_3":
				markdownBuilder.WriteString("### ")
				isHeading = true
			}

			if para.Bullet != nil {
				nestingLevel := 0
				if para.Bullet.NestingLevel > 0 {
					nestingLevel = int(para.Bullet.NestingLevel)
				}
				markdownBuilder.WriteString(strings.Repeat("  ", nestingLevel))
				markdownBuilder.WriteString("* ")
			}

			for _, elem := range para.Elements {
				if elem.TextRun != nil {
					text := elem.TextRun.Content
					style := elem.TextRun.TextStyle
					if isHeading && text == "\n" {
						continue
					}
					// 移除Markdown表格中不应存在的换行符
					text = strings.ReplaceAll(text, "\n", "")
					if style.Bold {
						text = "**" + text + "**"
					}
					if style.Italic {
						text = "*" + text + "*"
					}
					if style.Strikethrough {
						text = "~~" + text + "~~"
					}
					if style.Link != nil && style.Link.Url != "" {
						text = fmt.Sprintf("[%s](%s)", text, style.Link.Url)
					}
					markdownBuilder.WriteString(text)
				} else if elem.InlineObjectElement != nil {
					objId := elem.InlineObjectElement.InlineObjectId
					if inlineObj, ok := doc.InlineObjects[objId]; ok {
						_ = inlineObj.InlineObjectProperties.EmbeddedObject.ImageProperties
						placeholderURL := fmt.Sprintf("https://your-cdn.com/path/to/image-for-%s.png", objId)
						markdownBuilder.WriteString(fmt.Sprintf("\n\n![Image from Google Docs](%s)\n\n", placeholderURL))
					}
				}
			}

			if !isHeading {
				markdownBuilder.WriteString("\n")
			}
			markdownBuilder.WriteString("\n")
		} else if content.Table != nil { // --- 2. 新增：处理表格 (Table) ---
			table := content.Table
			if len(table.TableRows) > 0 {
				// 遍历行
				for i, row := range table.TableRows {
					var rowContent []string
					// 遍历单元格
					for _, cell := range row.TableCells {
						var cellTextBuilder strings.Builder
						// 提取单元格内的文本
						for _, cellContent := range cell.Content {
							if cellContent.Paragraph != nil {
								for _, paraElement := range cellContent.Paragraph.Elements {
									if paraElement.TextRun != nil {
										// 移除单元格文本中的换行符，防止破坏 Markdown 表格结构
										text := strings.ReplaceAll(paraElement.TextRun.Content, "\n", "")
										cellTextBuilder.WriteString(text)
									}
								}
							}
						}
						rowContent = append(rowContent, cellTextBuilder.String())
					}

					// 构建 Markdown 表格行
					markdownBuilder.WriteString("| " + strings.Join(rowContent, " | ") + " |\n")

					// 如果是第一行（表头），则在下面添加分隔线
					if i == 0 {
						var headerSeparators []string
						for range row.TableCells {
							headerSeparators = append(headerSeparators, "---")
						}
						markdownBuilder.WriteString("| " + strings.Join(headerSeparators, " | ") + " |\n")
					}
				}
				markdownBuilder.WriteString("\n") // 表格结束后添加一个换行符
			}
		}
	}

	return markdownBuilder.String(), nil
}

type wechatHTMLRenderer struct {
	tableHeaders  []string
	tableRowCount int
	inTableHeader bool
}

func (r *wechatHTMLRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindHeading, r.renderHeading)
	reg.Register(ast.KindParagraph, r.renderParagraph)
	reg.Register(ast.KindBlockquote, r.renderBlockquote)
	reg.Register(ast.KindCodeBlock, r.renderCodeBlock)
	reg.Register(ast.KindFencedCodeBlock, r.renderCodeBlock)
	reg.Register(ast.KindImage, r.renderImage)
	reg.Register(ast.KindList, r.renderList)
	reg.Register(ast.KindListItem, r.renderListItem)
	// Table renderer
	reg.Register(ext_ast.KindTable, r.renderTable)
	reg.Register(ext_ast.KindTableHeader, r.renderTableHeader)
	reg.Register(ext_ast.KindTableRow, r.renderTableRow)
	reg.Register(ext_ast.KindTableCell, r.renderTableCell)
}

// renderHeading 不再生成 h 标签，而是生成带有标题样式的 p 标签，以兼容微信编辑器
func (r *wechatHTMLRenderer) renderHeading(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*ast.Heading)
	if entering {
		var style string
		switch n.Level {
		case 1:
			style = styleH1
		case 2:
			style = styleH2
		case 3:
			style = styleH3
		default:
			style = styleH3
		}
		_, _ = w.WriteString(fmt.Sprintf("<p style=\"%s\">", style))
	} else {
		_, _ = w.WriteString("</p>\n")
	}
	return ast.WalkContinue, nil
}

func (r *wechatHTMLRenderer) renderParagraph(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		if _, ok := node.Parent().(*ast.ListItem); ok {
			return ast.WalkContinue, nil
		}
		_, _ = w.WriteString(fmt.Sprintf("<p style=\"%s\">", styleParagraph))
	} else {
		if _, ok := node.Parent().(*ast.ListItem); ok {
			return ast.WalkContinue, nil
		}
		_, _ = w.WriteString("</p>\n")
	}
	return ast.WalkContinue, nil
}

func (r *wechatHTMLRenderer) renderBlockquote(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		_, _ = w.WriteString(fmt.Sprintf("<blockquote style=\"%s\">", styleBlockquote))
	} else {
		_, _ = w.WriteString("</blockquote>\n")
	}
	return ast.WalkContinue, nil
}

func (r *wechatHTMLRenderer) renderCodeBlock(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		_, _ = w.WriteString(fmt.Sprintf("<pre style=\"%s\"><code>", styleCodeBlock))
		var lines [][]byte
		switch n := node.(type) {
		case *ast.CodeBlock:
			for i := 0; i < n.Lines().Len(); i++ {
				line := n.Lines().At(i)
				lines = append(lines, line.Value(source))
			}
		case *ast.FencedCodeBlock:
			for i := 0; i < n.Lines().Len(); i++ {
				line := n.Lines().At(i)
				lines = append(lines, line.Value(source))
			}
		}
		for _, line := range lines {
			w.Write(util.EscapeHTML(line))
		}
	} else {
		_, _ = w.WriteString("</code></pre>\n")
	}
	return ast.WalkSkipChildren, nil
}

func (r *wechatHTMLRenderer) renderImage(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*ast.Image)
	if entering {
		_, _ = w.WriteString(fmt.Sprintf("<img src=\"%s\" alt=\"%s\" style=\"%s\" />", n.Destination, n.Text(source), styleImage))
	}
	return ast.WalkSkipChildren, nil
}

func (r *wechatHTMLRenderer) renderList(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*ast.List)
	tag := "ul"
	style := styleUnorderedList
	if n.IsOrdered() {
		tag = "ol"
		style = styleOrderedList
	}
	if entering {
		_, _ = w.WriteString(fmt.Sprintf("<%s style=\"%s\">\n", tag, style))
	} else {
		_, _ = w.WriteString(fmt.Sprintf("</%s>\n", tag))
	}
	return ast.WalkContinue, nil
}

func (r *wechatHTMLRenderer) renderListItem(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		_, _ = w.WriteString(fmt.Sprintf("<li style=\"%s\">", styleListItem))
	} else {
		_, _ = w.WriteString("</li>\n")
	}
	return ast.WalkContinue, nil
}

// renderTable 初始化表格渲染，注入CSS动画
// renderTable 初始化表格渲染，重置所有状态
func (r *wechatHTMLRenderer) renderTable(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		// 重置所有状态，包括新的 inTableHeader 标志
		r.tableHeaders = []string{}
		r.tableRowCount = 0
		r.inTableHeader = false
		_, _ = w.WriteString(cssKeyframes)
		_, _ = w.WriteString(fmt.Sprintf("<div style=\"%s\">\n", styleTableWrapper))
	} else {
		_, _ = w.WriteString("</div>\n")
	}
	return ast.WalkContinue, nil
}

// renderTableHeader 的新作用：设置和取消状态旗帜
func (r *wechatHTMLRenderer) renderTableHeader(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		r.inTableHeader = true // 进入表头区域，升起旗帜
	} else {
		r.inTableHeader = false // 离开表头区域，放下旗帜
	}
	return ast.WalkContinue, nil // 继续遍历子节点 (TableRow -> TableCell)
}

// renderTableRow 保持不变，其父节点检查是可靠的
func (r *wechatHTMLRenderer) renderTableRow(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	// 这个检查是可靠的，因为 TableRow 的直接父节点就是 TableHeader
	if _, ok := node.Parent().(*ext_ast.TableHeader); ok {
		return ast.WalkContinue, nil // 跳过表头行容器的渲染
	}

	if entering {
		delay := float64(r.tableRowCount) * 0.1
		animatedStyle := fmt.Sprintf("%s animation-delay: %.2fs;", styleDataCard, delay)
		_, _ = w.WriteString(fmt.Sprintf("<div style=\"%s\">\n", animatedStyle))
		r.tableRowCount++
	} else {
		_, _ = w.WriteString("</div>\n")
	}
	return ast.WalkContinue, nil
}

// renderTableCell 使用新的状态旗帜进行判断
func (r *wechatHTMLRenderer) renderTableCell(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*ext_ast.TableCell)
	if !entering {
		return ast.WalkContinue, nil
	}

	// 提取单元格文本的逻辑保持不变
	var cellTextBuilder strings.Builder
	_ = ast.Walk(n, func(child ast.Node, entering bool) (ast.WalkStatus, error) {
		if textNode, ok := child.(*ast.Text); ok && entering {
			cellTextBuilder.Write(textNode.Segment.Value(source))
		}
		return ast.WalkContinue, nil
	})
	cellText := strings.TrimSpace(cellTextBuilder.String())

	// 【核心修正】用简单的布尔值检查，替代之前脆弱的父节点检查
	if r.inTableHeader {
		// 如果我们正处于表头区域，将单元格文本存入切片
		r.tableHeaders = append(r.tableHeaders, cellText)
	} else {
		// 否则，我们就在内容区域
		if cellText == "" {
			return ast.WalkSkipChildren, nil
		}

		cellIndex := 0
		for p := n.PreviousSibling(); p != nil; p = p.PreviousSibling() {
			cellIndex++
		}

		headerLabel := ""
		if cellIndex < len(r.tableHeaders) {
			headerLabel = r.tableHeaders[cellIndex]
		}

		isLast := true
		for p := n.NextSibling(); p != nil; p = p.NextSibling() {
			var nextCellTextBuilder strings.Builder
			_ = ast.Walk(p, func(child ast.Node, entering bool) (ast.WalkStatus, error) {
				if textNode, ok := child.(*ast.Text); ok && entering {
					nextCellTextBuilder.Write(textNode.Segment.Value(source))
				}
				return ast.WalkContinue, nil
			})
			if strings.TrimSpace(nextCellTextBuilder.String()) != "" {
				isLast = false
				break
			}
		}

		rowStyle := styleDataRow
		if isLast {
			rowStyle = styleDataRowLast
		}

		_, _ = w.WriteString(fmt.Sprintf("<p style=\"%s\">", rowStyle))
		_, _ = w.WriteString(fmt.Sprintf("<strong style=\"%s\">%s: </strong>", styleDataLabel, util.EscapeHTML([]byte(headerLabel))))
		w.Write(util.EscapeHTML([]byte(cellText)))
		_, _ = w.WriteString("</p>\n")
	}

	return ast.WalkSkipChildren, nil
}

func newMarkdown() goldmark.Markdown {
	customRenderer := &wechatHTMLRenderer{}
	return goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithRendererOptions(
			// 注册 customRenderer，设置优先级为 200（数值越大，优先级越高）
			renderer.WithNodeRenderers(
				util.Prioritized(customRenderer, 200),
			),
		),
	)
}

func main() {
	proxyAddr := flag.String("proxy", "", "SOCKS5 代理地址和端口, 例如: 127.0.0.1:1080")
	flag.Parse()

	if len(flag.Args()) < 1 {
		log.Fatalf("用法: go run . [--proxy <addr:port>] <documentId>\n例如: go run . --proxy 127.0.0.1:1080 YOUR_DOC_ID_HERE")
	}
	docId := flag.Args()[0]

	b, err := os.ReadFile("credentials.json")
	if err != nil {
		log.Fatalf("无法读取客户端密钥文件 (credentials.json): %v", err)
	}

	config, err := google.ConfigFromJSON(b, docs.DocumentsReadonlyScope)
	if err != nil {
		log.Fatalf("无法解析客户端密钥文件为配置: %v", err)
	}

	ctx := context.Background()
	if *proxyAddr != "" {
		fmt.Printf("使用 SOCKS5 代理: %s\n", *proxyAddr)
		dialer, err := proxy.SOCKS5("tcp", *proxyAddr, nil, proxy.Direct)
		if err != nil {
			log.Fatalf("无法创建 SOCKS5 代理拨号器: %v", err)
		}

		httpTransport := &http.Transport{}
		httpTransport.DialContext = dialer.(proxy.ContextDialer).DialContext

		httpClient := &http.Client{Transport: httpTransport}
		ctx = context.WithValue(ctx, oauth2.HTTPClient, httpClient)
	}

	client := getClient(ctx, config)
	srv, err := docs.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		log.Fatalf("无法创建 Docs 服务: %v", err)
	}

	fmt.Println("正在从 Google Docs 获取并解析文档...")
	markdownContent, err := processDocument(srv, docId)
	if err != nil {
		log.Fatalf("处理文档失败: %v", err)
	}

	// print markdown content
	fmt.Println(markdownContent)

	md := newMarkdown()

	var htmlBuffer bytes.Buffer
	htmlBuffer.WriteString(fmt.Sprintf("<div style=\"%s\">\n", styleBody))
	if err := md.Convert([]byte(markdownContent), &htmlBuffer); err != nil {
		log.Fatalf("Markdown 转换为 HTML 失败: %v", err)
	}
	htmlBuffer.WriteString("</div>")

	outputFile := "output.html"
	err = os.WriteFile(outputFile, htmlBuffer.Bytes(), 0644)
	if err != nil {
		log.Fatalf("写入 HTML 文件失败: %v", err)
	}

	fmt.Println("\n=======================================================")
	fmt.Printf("🎉 转换成功！结果已保存到 %s\n", outputFile)
	fmt.Println("\n下一步操作:")
	fmt.Println("1. 打开 output.html 文件，你会看到渲染后的效果。")
	fmt.Println("2. 【重要】检查文件中的图片 URL，它们是占位符。你需要：")
	fmt.Println("   a. 将 Google Doc 中的图片手动保存下来。")
	fmt.Println("   b. 上传到你自己的服务器、CDN 或图床（如腾讯云 COS）。")
	fmt.Println("   c. 将 `output.html` 中 `https://your-cdn.com/...` 这样的占位符 URL 替换为真实的图片 URL。")
	fmt.Println("3. 用浏览器打开修改后的 `output.html` 文件，全选 (Ctrl+A / Cmd+A) 并复制 (Ctrl+C / Cmd+C)。")
	fmt.Println("4. 粘贴到微信公众号后台的编辑器中。")
	fmt.Println("=======================================================")

	ex, err := os.Executable()
	if err == nil {
		fmt.Printf("\n文件保存在: %s\n", filepath.Dir(ex))
	}
}
