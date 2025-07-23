# Google Docs to WeChat Formatter (Google 文档转微信公众号格式化工具)

这是一个使用 Go 语言编写的命令行工具，旨在将 Google Docs 文档转换为排版精美、适配微信公众号编辑器的 HTML 文件。它通过 Google Docs API 获取文档内容，将其转换为 Markdown，最后渲染成带有内联样式的 HTML，可以直接复制粘贴到微信后台。

## 功能特性

-   **Google Docs 集成**: 使用文档 ID 直接从 Google Docs 获取内容。
-   **Markdown 转换**: 智能地将 Google Docs 的格式（标题、粗体、斜体、列表、链接等）转换为 Markdown。
-   **微信优化 HTML**: 将 Markdown 渲染为带有内联 CSS 样式的 HTML，这些样式专门为兼容微信编辑器而设计，并且可自定义。
-   **图片占位符**: 为文档中的图片自动生成占位符 `<img>` 标签，方便你替换为自己的 CDN 或图床链接。
-   **OAuth 2.0 认证**: 安全地处理 Google API 的认证流程，并将凭证（token）保存以备将来使用，无需重复授权。
-   **代理支持**: 内置 SOCKS5 代理支持，方便在有网络限制环境（如中国大陆）的用户使用。

## 环境准备

1.  **Go 环境**: 你的系统需要安装 Go (版本 1.16 或更高)。
2.  **Google Cloud 项目**: 你需要一个 Google 账户，并在 Google Cloud Platform 中创建一个项目。
3.  **本地 SOCKS5 代理 (可选)**: 如果你所在的地区访问 Google 服务受限，你需要一个本地运行的 SOCKS5 代理。

## 安装与配置

### 第 1 步：启用 Google Docs API 并创建凭证

1.  访问 [Google Cloud Console](https://console.cloud.google.com/)。
2.  创建一个新项目或选择一个现有项目。
3.  进入 **API 和服务 > 库**。搜索 "Google Docs API" 并点击 **启用**。
4.  进入 **API 和服务 > 凭据**。
5.  点击 **+ 创建凭据** 并选择 **OAuth 客户端 ID**。
6.  选择 **桌面应用** 作为应用类型，并给它起个名字（例如 "GDoc-to-WeChat-Converter"）。
7.  点击 **创建**。一个弹窗会显示你的客户端 ID 和客户端密钥。点击 **下载 JSON**。
8.  将下载的文件重命名为 `credentials.json`，并将其放置在 Go 程序所在的同一目录下。

### 第 2 步：配置测试用户

由于你的应用处于“测试”状态，你必须明确地将你的 Google 账户添加为“测试用户”，否则无法授权。

1.  在 Google Cloud Console，进入 **API 和服务 > OAuth 同意屏幕**。
2.  在“OAuth 同意屏幕”页面，你将看到应用的状态是“测试”。
3.  在此页面下方，找到名为 **“测试用户” (Test users)** 的板块。
4.  点击 **“+ ADD USERS”** 按钮。
5.  在弹出的输入框中，输入你**稍后将在浏览器中用于授权此应用的 Google 账户的电子邮件地址**（也就是你 Google Docs 所在的那个账户）。
6.  点击 **“保存” (SAVE)**。

### 第 3 步：获取 Go 程序

克隆此仓库或将 `main.go` 文件的代码保存到一个新目录中。

```bash
# 示例目录结构
mkdir gdoc-to-wechat
cd gdoc-to-wechat
# 在此目录下创建 main.go 和 credentials.json 文件
```

然后，获取必要的 Go 依赖包：

```bash
go mod init gdoc-to-wechat
go get github.com/yuin/goldmark google.golang.org/api/docs/v1 golang.org/x/oauth2/google golang.org/x/net/proxy
```

## 如何使用

### 首次运行与授权

第一次运行本工具时，你需要授权它访问你的 Google Docs。

1.  从你的 Google Doc 的 URL 中找到 **文档 ID**。对于 `https://docs.google.com/document/d/文档ID/edit`，ID 就是中间那段长字符串。

2.  在终端中运行程序。

    **如果你不需要代理：**
    ```bash
    go run . 你的文档ID
    ```

    **如果你在中国大陆等需要代理的环境：**
    (请将 `1080` 替换为你的实际代理端口号)
    ```bash
    go run . --proxy 127.0.0.1:1080 你的文档ID
    ```

3.  程序会打印出一个 URL。复制它并粘贴到你的网页浏览器中。

4.  登录你已添加为测试用户的那个 Google 账户。

5.  你可能会看到一个“Google 未验证此应用”的警告页面。这是正常的，因为应用未提交官方审核。点击“高级”，然后选择“继续前往 [你的应用名称] (不安全)”。

6.  点击 **允许** 来授予权限。

7.  Google 会在页面上显示一串授权码。复制这串代码。

8.  将代码粘贴回你的终端，然后按回车键。

工具现在会自动获取文档、转换并保存为 `output.html`。同时，目录下会生成一个 `token.json` 文件。**之后的所有运行都无需再重复此授权步骤**。

### 后续运行

以后再使用时，只需简单地再次运行命令即可。工具会使用已保存的 `token.json`，不会再要求授权。

**无代理：**
```bash
go run . 另一个文档ID
```

**有代理：**
```bash
go run . --proxy 127.0.0.1:1080 另一个文档ID
```

## 发布到微信公众号的工作流

1.  运行工具生成 `output.html` 文件。
2.  **关键步骤：手动处理图片。** 生成的 `output.html` 包含了 `https://your-cdn.com/...` 这样的占位符链接。
    a. 从你的 Google Doc 中手动保存图片。
    b. 将图片上传到你自己的 CDN 或图床（如腾讯云 COS、阿里云 OSS 等）。
    c. 用文本编辑器打开 `output.html`，将里面的占位符链接替换为你真实的图片 URL。
3.  用浏览器（如 Chrome 或 Firefox）打开修改后的 `output.html` 文件。
4.  全选 (Ctrl+A 或 Cmd+A) 并复制 (Ctrl+C 或 Cmd+C) 页面内容。
5.  直接粘贴到微信公众号后台的编辑器中。文章的格式和样式应该会被完整保留。

## 自定义样式

你可以通过编辑 `main.go` 文件顶部的样式常量来轻松定制文章的外观。

```go
// --- 可配置的样式 ---
// 你可以根据自己的公众号风格修改这些内联样式
const (
	styleBody          = `padding: 16px; font-size: 16px; line-height: 1.75; color: #333;`
	styleH1            = `margin-bottom: 20px; font-size: 24px; font-weight: bold; text-align: center;`
	styleH2            = `margin-top: 25px; margin-bottom: 15px; font-size: 20px; font-weight: bold; border-bottom: 1px solid #ddd; padding-bottom: 5px;`
	// ... 其他样式
)
```

## 故障排查

-   **`错误 403： access_denied`**: 这个错误意味着你用于授权的 Google 账户没有被添加到项目的“测试用户”列表中。请遵循 **安装与配置** 的 **第 2 步** 将其添加。
-   **`i/o timeout`**: 这是一个网络连接超时错误，通常是因为你所在的地区访问 Google 服务受限。请使用 `--proxy` 标志来通过你的本地 SOCKS5 代理发出请求。
    ```bash
    go run . --proxy 127.0.0.1:1080 你的文档ID
    ```
-   **`cannot find package "golang.org/x/net/proxy"`**: 你忘记了下载依赖包。请运行 `go get golang.org/x/net/proxy`。