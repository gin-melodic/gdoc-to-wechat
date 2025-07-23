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
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"
	"golang.org/x/net/proxy"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/docs/v1"
	"google.golang.org/api/option"
)

// --- 可配置的样式 ---
// 你可以根据自己的公众号风格修改这些内联样式
const (
	styleBody          = `padding: 16px; letter-spacing: 0.544px; font-size: 16px; line-height: 1.75; color: #333;`
	styleH1            = `margin-top: 30px; margin-bottom: 20px; font-size: 24px; font-weight: bold; line-height: 1.4; text-align: center;`
	styleH2            = `margin-top: 25px; margin-bottom: 15px; font-size: 20px; font-weight: bold; line-height: 1.4; border-bottom: 2px solid #f2f2f2; padding-bottom: 5px;`
	styleH3            = `margin-top: 20px; margin-bottom: 12px; font-size: 18px; font-weight: bold; line-height: 1.4;`
	styleParagraph     = `margin-top: 1em; margin-bottom: 1em;`
	styleBlockquote    = `padding: 10px 20px; margin: 20px 0; background-color: #f8f8f8; border-left: 4px solid #d1d1d1; color: #666;`
	styleCodeBlock     = `display: block; overflow-x: auto; padding: 1em; background: #23241f; color: #f8f8f2; margin: 20px 0; border-radius: 5px; font-family: 'Courier New', Courier, monospace;`
	styleImage         = `max-width: 100%; height: auto; display: block; margin: 20px auto; border-radius: 4px; box-shadow: 0 4px 8px rgba(0,0,0,0.1);`
	styleUnorderedList = `margin: 1em 0; padding-left: 25px;`
	styleOrderedList   = `margin: 1em 0; padding-left: 25px;`
	styleListItem      = `margin-bottom: 0.5em;`
)

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

// processDocument 是核心处理函数，增加了忽略标题和参考文献的功能
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
		if content.Paragraph != nil {
			para := content.Paragraph

			// 1. 忽略文档标题 (style "TITLE")
			if para.ParagraphStyle.NamedStyleType == "TITLE" {
				fmt.Println("已忽略文档标题。")
				continue
			}

			// 提取段落纯文本内容，用于判断是否为“参考文献”等停止标记
			var paraTextBuilder strings.Builder
			for _, elem := range para.Elements {
				if elem.TextRun != nil {
					paraTextBuilder.WriteString(elem.TextRun.Content)
				}
			}
			paraText := strings.TrimSpace(paraTextBuilder.String())

			// 2. 如果段落文本是停止词，则中断后续所有处理
			if stopHeadings[paraText] {
				fmt.Printf("\n检测到章节 “%s”，已停止后续内容转换。\n", paraText)
				break // 中断 for 循环，不再处理任何后续内容
			}

			// --- 如果不是需要忽略的内容，则按原逻辑处理 ---
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
		}
	}

	return markdownBuilder.String(), nil
}

type wechatHTMLRenderer struct{}

func (r wechatHTMLRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindHeading, r.renderHeading)
	reg.Register(ast.KindParagraph, r.renderParagraph)
	reg.Register(ast.KindBlockquote, r.renderBlockquote)
	reg.Register(ast.KindCodeBlock, r.renderCodeBlock)
	reg.Register(ast.KindFencedCodeBlock, r.renderCodeBlock)
	reg.Register(ast.KindImage, r.renderImage)
	reg.Register(ast.KindList, r.renderList)
	reg.Register(ast.KindListItem, r.renderListItem)
}

// renderHeading 不再生成 h 标签，而是生成带有标题样式的 p 标签，以兼容微信编辑器
func (r wechatHTMLRenderer) renderHeading(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
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

func (r wechatHTMLRenderer) renderParagraph(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
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

func (r wechatHTMLRenderer) renderBlockquote(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		_, _ = w.WriteString(fmt.Sprintf("<blockquote style=\"%s\">", styleBlockquote))
	} else {
		_, _ = w.WriteString("</blockquote>\n")
	}
	return ast.WalkContinue, nil
}

func (r wechatHTMLRenderer) renderCodeBlock(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
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

func (r wechatHTMLRenderer) renderImage(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*ast.Image)
	if entering {
		_, _ = w.WriteString(fmt.Sprintf("<img src=\"%s\" alt=\"%s\" style=\"%s\" />", n.Destination, n.Text(source), styleImage))
	}
	return ast.WalkSkipChildren, nil
}

func (r wechatHTMLRenderer) renderList(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
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

func (r wechatHTMLRenderer) renderListItem(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		_, _ = w.WriteString(fmt.Sprintf("<li style=\"%s\">", styleListItem))
	} else {
		_, _ = w.WriteString("</li>\n")
	}
	return ast.WalkContinue, nil
}

func newMarkdown() goldmark.Markdown {
	return goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithRendererOptions(
			// 注册 customRenderer，设置优先级为 200（数值越大，优先级越高）
			renderer.WithNodeRenderers(
				util.Prioritized(wechatHTMLRenderer{}, 200),
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
