# Google Docs to WeChat Formatter

[中文文档](README_CN.md)

This is a command-line tool written in Go that converts a Google Docs document into a clean, WeChat-friendly HTML file. It fetches the document content using the Google Docs API, converts it to Markdown, and then renders it into HTML with inline styles optimized for the WeChat public account editor.

## Features

-   **Google Docs Integration**: Directly fetches content from a Google Doc using its Document ID.
-   **Markdown Conversion**: Intelligently converts Google Docs formatting (headings, bold, italics, lists, links) into Markdown.
-   **WeChat-Optimized HTML**: Renders Markdown to HTML with customizable inline CSS styles that are compatible with the WeChat editor.
-   **Image Placeholders**: Automatically generates placeholder `<img>` tags for images, making it easy to replace them with your CDN/image hosting URLs.
--   **OAuth 2.0 Handling**: Securely handles Google API authentication, storing the token for future use.
-   **Proxy Support**: Built-in support for using a SOCKS5 proxy for users in network-restricted environments (e.g., mainland China).

## Prerequisites

1.  **Go**: You need to have Go (version 1.16 or later) installed on your system.
2.  **Google Cloud Project**: You need a Google account and a project set up in the Google Cloud Platform.
3.  **Local SOCKS5 Proxy (Optional)**: If you are in a region with restricted access to Google services, you will need a local SOCKS5 proxy running.

## Setup Instructions

### Step 1: Enable Google Docs API & Create Credentials

1.  Go to the [Google Cloud Console](https://console.cloud.google.com/).
2.  Create a new project or select an existing one.
3.  Go to **APIs & Services > Library**. Search for "Google Docs API" and click **Enable**.
4.  Go to **APIs & Services > Credentials**.
5.  Click **+ CREATE CREDENTIALS** and choose **OAuth client ID**.
6.  Select **Desktop app** as the Application type and give it a name (e.g., "GDoc-to-WeChat-Converter").
7.  Click **Create**. A pop-up will show your Client ID and Client Secret. Click **DOWNLOAD JSON**.
8.  Rename the downloaded file to `credentials.json` and place it in the same directory as the Go program.

### Step 2: Configure Test Users

Since your app will be in "Testing" mode, you must explicitly authorize your Google account to use it.

1.  In the Google Cloud Console, go to **APIs & Services > OAuth consent screen**.
2.  Under the **Test users** section, click **+ ADD USERS**.
3.  Enter the email address of the Google account you will use to authorize the application (the one that owns the Google Docs).
4.  Click **SAVE**.

### Step 3: Get the Go Program

Clone this repository or save the `main.go` code into a file in a new directory.

```bash
# Example directory setup
mkdir gdoc-to-wechat
cd gdoc-to-wechat
# Create main.go and credentials.json inside this directory
```

Then, fetch the necessary Go dependencies:

```bash
go mod init gdoc-to-wechat
go get github.com/yuin/goldmark google.golang.org/api/docs/v1 golang.org/x/oauth2/google golang.org/x/net/proxy
```

## How to Use

### First-Time Authorization

The first time you run the tool, you'll need to authorize it to access your Google Docs.

1.  Find the **Document ID** from your Google Doc's URL. For `https://docs.google.com/document/d/DOCUMENT_ID/edit`, the ID is the long string in the middle.

2.  Run the program from your terminal.

    **If you don't need a proxy:**
    ```bash
    go run . YOUR_DOCUMENT_ID
    ```

    **If you are behind a firewall and need a SOCKS5 proxy:**
    (Replace `1080` with your actual proxy port)
    ```bash
    go run . --proxy 127.0.0.1:1080 YOUR_DOCUMENT_ID
    ```

3.  The program will print a URL. Copy it and paste it into your web browser.

4.  Log in to the Google account you added as a test user.

5.  You might see a warning screen saying "Google hasn't verified this app." This is expected. Click "Advanced" and then "Go to [your-app-name] (unsafe)".

6.  Click **Allow** to grant permission.

7.  Google will display an authorization code. Copy this code.

8.  Paste the code back into your terminal and press Enter.

The tool will now fetch the document, convert it, and save it as `output.html`. A `token.json` file will also be created. **You will not need to repeat this authorization process again.**

### Subsequent Runs

For all future uses, simply run the command again. The tool will use the saved `token.json` and will not ask for authorization.

**Without proxy:**
```bash
go run . ANOTHER_DOCUMENT_ID
```

**With proxy:**
```bash
go run . --proxy 127.0.0.1:1080 ANOTHER_DOCUMENT_ID
```

## Workflow for Publishing to WeChat

1.  Run the tool to generate `output.html`.
2.  **Crucially, you must manually handle images.** The generated `output.html` contains placeholder URLs like `https://your-cdn.com/...`.
    a. Manually save the images from your Google Doc.
    b. Upload them to your own CDN or an image hosting service (like Tencent Cloud COS, etc.).
    c. Open `output.html` in a text editor and replace the placeholder image URLs with your real URLs.
3.  Open the final, modified `output.html` file in a web browser (like Chrome or Firefox).
4.  Select all content (Ctrl+A or Cmd+A) and copy it (Ctrl+C or Cmd+C).
5.  Paste the content directly into the WeChat public account editor. The formatting and styles should be preserved.

## Customization

You can easily customize the look and feel of your articles by editing the style constants at the top of the `main.go` file.

```go
// --- Configuration Styles ---
// You can modify these inline styles to match your WeChat public account style
const (
	styleBody          = `padding: 16px; font-size: 16px; line-height: 1.75; color: #333;`
	styleH1            = `margin-bottom: 20px; font-size: 24px; font-weight: bold; text-align: center;`
	styleH2            = `margin-top: 25px; margin-bottom: 15px; font-size: 20px; font-weight: bold; border-bottom: 1px solid #ddd; padding-bottom: 5px;`
	// ... and so on
)
```

## Troubleshooting

-   **`Error 403: access_denied`**: This means the Google account you're trying to authorize with is not listed as a "Test user" in your Google Cloud project's OAuth consent screen. Follow **Step 2** of the setup instructions to add it.
-   **`i/o timeout`**: This is a network connection error, usually because you are in a region with restricted access to Google services. Use the `--proxy` flag to route traffic through your local SOCKS5 proxy.
    ```bash
    go run . --proxy 127.0.0.1:1080 YOUR_DOCUMENT_ID
    ```
-   **`cannot find package "golang.org/x/net/proxy"`**: You forgot to download the dependency. Run `go get golang.org/x/net/proxy`.