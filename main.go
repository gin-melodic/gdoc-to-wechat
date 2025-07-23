package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/renderer"

	// "github.com/yuin/goldmark/renderer/html" // 不再直接需要
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

// getClient 使用 OAuth 2.0 配置来检索 token，如果需要，会提示用户授权。
func getClient(ctx context.Context, config *oauth2.Config) *http.Client {
	tokFile := "token.json"
	tok, err := tokenFromFile(tokFile)
	if err != nil {
		tok = getTokenFromWeb(ctx, config) // 修改: 传递 context
		saveToken(tokFile, tok)
	}
	// 修改: 使用传入的 context 来创建 client，这样 oauth2 库就会使用我们配置了代理的 http.Client
	return config.Client(ctx, tok)
}

// getTokenFromWeb 从 web 请求一个新的 token。
func getTokenFromWeb(ctx context.Context, config *oauth2.Config) *oauth2.Token {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("请在浏览器中打开以下链接进行授权: \n%v\n", authURL)
	fmt.Print("输入授权码: ")

	var authCode string
	if _, err := fmt.Scan(&authCode); err != nil {
		log.Fatalf("无法读取授权码: %v", err)
	}

	// 修改: 传递 context 给 Exchange 方法
	tok, err := config.Exchange(ctx, authCode)
	if err != nil {
		log.Fatalf("无法从授权码换取 token: %v", err)
	}
	return tok
}

// tokenFromFile 从文件中读取 token。
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

// saveToken 将 token 保存到文件。
func saveToken(path string, token *oauth2.Token) {
	fmt.Printf("保存凭证文件到: %s\n", path)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Fatalf("无法缓存 oauth token: %v", err)
	}
	defer f.Close()
	json.NewEncoder(f).Encode(token)
}

// processDocument 是核心处理函数
func processDocument(srv *docs.Service, docId string) (string, error) {
	doc, err := srv.Documents.Get(docId).Do()
	if err != nil {
		return "", fmt.Errorf("无法获取文档: %v", err)
	}

	var markdownBuilder strings.Builder

	// 遍历文档内容元素
	for _, content := range doc.Body.Content {
		if content.Paragraph != nil {
			// 处理段落
			para := content.Paragraph
			isHeading := false
			// 根据段落样式确定标题级别
			// Google Docs 默认样式: "NORMAL_TEXT", "TITLE", "SUBTITLE", "HEADING_1", ...
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

			// 检查是否是列表项
			if para.Bullet != nil {
				// 获取列表项的缩进级别
				nestingLevel := 0
				if para.Bullet.NestingLevel > 0 {
					nestingLevel = int(para.Bullet.NestingLevel)
				}
				// 添加 Markdown 列表标记
				markdownBuilder.WriteString(strings.Repeat("  ", nestingLevel))
				markdownBuilder.WriteString("* ") // 简单起见，所有列表都转为无序列表
			}

			// 处理段落中的文本元素
			for _, elem := range para.Elements {
				if elem.TextRun != nil {
					text := elem.TextRun.Content
					style := elem.TextRun.TextStyle

					// 忽略标题中的换行符，因为GDocs API会在标题后加一个\n
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
					// 链接
					if style.Link != nil && style.Link.Url != "" {
						text = fmt.Sprintf("[%s](%s)", text, style.Link.Url)
					}
					markdownBuilder.WriteString(text)
				} else if elem.InlineObjectElement != nil {
					// 处理图片
					objId := elem.InlineObjectElement.InlineObjectId
					if _, ok := doc.InlineObjects[objId]; ok {
						// Google Docs 的 contentUri 是临时的，无法直接使用
						// 我们在这里生成一个占位符，提示用户替换
						// 【重要】用户需要将图片上传到自己的 CDN/图床，然后替换这里的 URL
						placeholderURL := fmt.Sprintf("https://your-cdn.com/path/to/image-for-%s.png", objId)
						markdownBuilder.WriteString(fmt.Sprintf("\n\n![Image from Google Docs](%s)\n\n", placeholderURL))
					}
				}
			}

			// 确保非标题段落后有换行符
			if !isHeading {
				markdownBuilder.WriteString("\n")
			}
			markdownBuilder.WriteString("\n") // 每个段落块后添加空行
		}
	}

	return markdownBuilder.String(), nil
}

// --- Goldmark 自定义渲染 ---

// wechatHTMLRenderer 是一个自定义的节点渲染器，用于覆盖默认的HTML输出
type wechatHTMLRenderer struct {
	// 这个结构体是空的，因为它只是一组方法的集合
}

// RegisterFuncs 实现了 renderer.NodeRenderer 接口。
// Goldmark 会调用此方法来注册我们的自定义渲染函数。
func (r *wechatHTMLRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindHeading, r.renderHeading)
	reg.Register(ast.KindParagraph, r.renderParagraph)
	reg.Register(ast.KindBlockquote, r.renderBlockquote)
	reg.Register(ast.KindCodeBlock, r.renderCodeBlock)
	reg.Register(ast.KindFencedCodeBlock, r.renderCodeBlock) // 同时处理 FencedCodeBlock
	reg.Register(ast.KindImage, r.renderImage)
	reg.Register(ast.KindList, r.renderList)
	reg.Register(ast.KindListItem, r.renderListItem)
}

// wechatExtension 是一个 Goldmark 扩展，它将我们的自定义渲染器应用进去
type wechatExtension struct{}

// Extend 实现了 goldmark.Extender 接口
func (e *wechatExtension) Extend(m goldmark.Markdown) {
	// 使用 WithNodeRenderers 选项添加我们的渲染器。
	// util.Prioritized 确保我们的渲染器具有高优先级，从而覆盖默认的渲染器。
	m.Renderer().AddOptions(renderer.WithNodeRenderers(
		util.Prioritized(&wechatHTMLRenderer{}, 1000),
	))
}

// 以下渲染函数现在是 *wechatHTMLRenderer 的方法

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
			style = styleH3 // 默认 H3 样式
		}
		_, _ = w.WriteString(fmt.Sprintf("<h%d style=\"%s\">", n.Level, style))
	} else {
		_, _ = w.WriteString(fmt.Sprintf("</h%d>\n", n.Level))
	}
	return ast.WalkContinue, nil
}

func (r *wechatHTMLRenderer) renderParagraph(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		// 如果段落的父节点是列表项，则不添加 P 标签，避免额外的间距
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
			// HTML-escape the content
			w.Write(util.EscapeHTML(line))
		}
	} else {
		_, _ = w.WriteString("</code></pre>\n")
	}
	// 返回 WalkSkipChildren 是因为我们已经手动处理了所有子节点（代码行）
	return ast.WalkSkipChildren, nil
}

func (r *wechatHTMLRenderer) renderImage(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*ast.Image)
	if entering {
		// 微信图片样式
		_, _ = w.WriteString(fmt.Sprintf("<img src=\"%s\" alt=\"%s\" style=\"%s\" />", n.Destination, n.Text(source), styleImage))
	}
	// 图片是自闭合标签，不需要处理 leaving
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

func main() {
	// --- 新增: 解析命令行参数 ---
	proxyAddr := flag.String("proxy", "", "SOCKS5 代理地址和端口, 例如: 127.0.0.1:1080")
	flag.Parse()

	// flag.Parse() 会处理所有标志，剩下的非标志参数在 flag.Args() 中
	if len(flag.Args()) < 1 {
		log.Fatalf("用法: go run . [--proxy <addr:port>] <documentId>\n例如: go run . --proxy 127.0.0.1:1080 YOUR_DOC_ID_HERE")
	}
	docId := flag.Args()[0]
	// --- 参数解析结束 ---

	// 读取凭证
	b, err := ioutil.ReadFile("credentials.json")
	if err != nil {
		log.Fatalf("无法读取客户端密钥文件 (credentials.json): %v", err)
	}

	// 配置 OAuth2
	config, err := google.ConfigFromJSON(b, docs.DocumentsReadonlyScope)
	if err != nil {
		log.Fatalf("无法解析客户端密钥文件为配置: %v", err)
	}

	// --- 根据代理参数创建和配置 context ---
	ctx := context.Background()
	if *proxyAddr != "" {
		fmt.Printf("使用 SOCKS5 代理: %s\n", *proxyAddr)
		// 创建一个 SOCKS5 拨号器
		dialer, err := proxy.SOCKS5("tcp", *proxyAddr, nil, proxy.Direct)
		if err != nil {
			log.Fatalf("无法创建 SOCKS5 代理拨号器: %v", err)
		}

		// 创建一个配置了代理的 http.Transport
		httpTransport := &http.Transport{}
		httpTransport.DialContext = dialer.(proxy.ContextDialer).DialContext

		// 创建一个使用该 transport 的 http.Client
		httpClient := &http.Client{Transport: httpTransport}

		// 将这个自定义的 http.Client 放入 context 中
		// oauth2 库和 google.golang.org/api 库会自动检测并使用这个 client
		ctx = context.WithValue(ctx, oauth2.HTTPClient, httpClient)
	}
	// --- context 配置结束 ---

	client := getClient(ctx, config)

	// 创建 Google Docs 服务
	srv, err := docs.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		log.Fatalf("无法创建 Docs 服务: %v", err)
	}

	fmt.Println("正在从 Google Docs 获取并解析文档...")

	// 处理文档，得到 Markdown 中间产物
	markdownContent, err := processDocument(srv, docId)
	if err != nil {
		log.Fatalf("处理文档失败: %v", err)
	}

	// --- 将 Markdown 转换为微信格式的 HTML ---
	md := goldmark.New(
		// 使用我们的自定义扩展来应用样式
		goldmark.WithExtensions(
			&wechatExtension{},
		),
	)

	var htmlBuffer bytes.Buffer
	// 在所有内容外部包裹一个带样式的 div
	htmlBuffer.WriteString(fmt.Sprintf("<div style=\"%s\">\n", styleBody))

	if err := md.Convert([]byte(markdownContent), &htmlBuffer); err != nil {
		log.Fatalf("Markdown 转换为 HTML 失败: %v", err)
	}
	htmlBuffer.WriteString("</div>")

	// 保存到文件
	outputFile := "output.html"
	err = ioutil.WriteFile(outputFile, htmlBuffer.Bytes(), 0644)
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

	// 打印可执行文件所在路径
	ex, err := os.Executable()
	if err == nil {
		fmt.Printf("\n文件保存在: %s\n", filepath.Dir(ex))
	}
}
