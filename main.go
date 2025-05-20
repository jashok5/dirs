package main

import (
	"fmt"
	"html/template"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// FileInfo 文件信息结构体
type FileInfo struct {
	Name    string
	Size    int64
	ModTime string
	IsDir   bool
	Path    string
}

// 目录列表模板
const dirListTemplate = `
<!DOCTYPE html>
<html>
<head>
    <title>文件列表</title>
    <meta charset="utf-8">
    <style>
        body {
            font-family: Arial, sans-serif;
            margin: 20px;
        }
        table {
            border-collapse: collapse;
            width: 100%;
        }
        th, td {
            padding: 8px;
            text-align: left;
            border-bottom: 1px solid #ddd;
        }
        tr:hover {
            background-color: #f5f5f5;
        }
        th {
            background-color: #4CAF50;
            color: white;
        }
        a {
            text-decoration: none;
            color: #0066cc;
        }
        a:hover {
            text-decoration: underline;
        }
        .back {
            margin-bottom: 10px;
        }
    </style>
</head>
<body>
    <h1>文件列表: {{.Path}}</h1>
    {{if ne .Path "/"}}
    <div class="back"><a href="..">返回上级目录</a></div>
    {{end}}
    <table>
        <tr>
            <th>名称</th>
            <th>大小</th>
            <th>修改时间</th>
        </tr>
        {{range .Files}}
        <tr>
            <td><a href="{{.Path}}">{{.Name}}{{if .IsDir}}/{{end}}</a></td>
            <td>{{if .IsDir}}-{{else}}{{.Size}} 字节{{end}}</td>
            <td>{{.ModTime}}</td>
        </tr>
        {{end}}
    </table>
</body>
</html>
`

// 获取本机的局域网IP地址
func getLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "localhost"
	}

	for _, address := range addrs {
		// 检查IP地址类型
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}

	return "localhost"
}

func main() {
	// 获取程序所在的绝对路径
	execPath, err := os.Executable()
	if err != nil {
		fmt.Println("获取程序路径失败:", err)
		return
	}

	// 获取程序所在的目录
	rootDir := filepath.Dir(execPath)

	// 切换工作目录到程序所在目录
	err = os.Chdir(rootDir)
	if err != nil {
		fmt.Println("切换工作目录失败:", err)
		return
	}

	fmt.Println("文件服务器根目录:", rootDir)

	// 获取本机局域网IP地址
	localIP := getLocalIP()

	// 创建HTTP服务器
	http.HandleFunc("/", handleFileServer)

	fmt.Printf("文件服务器已启动，访问 http://%s:8899\n", localIP)
	err = http.ListenAndServe(":8899", nil)
	if err != nil {
		fmt.Println("服务器启动失败:", err)
	}
}

func handleFileServer(w http.ResponseWriter, r *http.Request) {
	// 获取请求的路径
	urlPath := r.URL.Path

	// 将URL路径转换为本地文件系统路径
	// 使用当前工作目录（已设置为程序所在目录）
	localPath := "." + urlPath

	// 规范化路径，防止目录遍历攻击
	localPath = filepath.Clean(localPath)

	// 获取程序自身的文件名
	execPath, _ := os.Executable()
	execName := filepath.Base(execPath)

	// 检查请求的是否为程序自身
	if filepath.Base(localPath) == execName {
		http.Error(w, "无权访问此文件", http.StatusForbidden)
		return
	}

	// 获取文件信息
	fileInfo, err := os.Stat(localPath)
	if err != nil {
		http.Error(w, "文件或目录不存在", http.StatusNotFound)
		return
	}

	// 如果是目录，显示目录列表
	if fileInfo.IsDir() {
		renderDirList(w, r, localPath, urlPath, execName)
		return
	}

	// 如果是文件，提供下载
	file, err := os.Open(localPath)
	if err != nil {
		http.Error(w, "无法打开文件", http.StatusInternalServerError)
		return
	}
	defer file.Close()

	// 设置Content-Disposition头，使浏览器下载文件而不是显示
	w.Header().Set("Content-Disposition", "attachment; filename="+filepath.Base(localPath))
	w.Header().Set("Content-Type", "application/octet-stream")

	// 将文件内容复制到响应
	_, err = io.Copy(w, file)
	if err != nil {
		http.Error(w, "文件传输错误", http.StatusInternalServerError)
		return
	}
}

func renderDirList(w http.ResponseWriter, r *http.Request, localPath, urlPath, execName string) {
	// 读取目录内容
	dir, err := os.Open(localPath)
	if err != nil {
		http.Error(w, "无法打开目录", http.StatusInternalServerError)
		return
	}
	defer dir.Close()

	// 获取目录中的所有文件和子目录
	fileInfos, err := dir.Readdir(-1)
	if err != nil {
		http.Error(w, "无法读取目录内容", http.StatusInternalServerError)
		return
	}

	// 创建文件信息列表
	var files []FileInfo
	for _, info := range fileInfos {
		// 跳过隐藏文件和程序自身
		if strings.HasPrefix(info.Name(), ".") || info.Name() == execName {
			continue
		}

		// 构建文件路径
		filePath := filepath.Join(urlPath, info.Name())
		// 确保URL路径使用正斜杠
		filePath = strings.Replace(filePath, "\\", "/", -1)

		files = append(files, FileInfo{
			Name:    info.Name(),
			Size:    info.Size(),
			ModTime: info.ModTime().Format("2006-01-02 15:04:05"),
			IsDir:   info.IsDir(),
			Path:    filePath,
		})
	}

	// 按照目录在前，文件在后的顺序排序
	sort.Slice(files, func(i, j int) bool {
		if files[i].IsDir && !files[j].IsDir {
			return true
		}
		if !files[i].IsDir && files[j].IsDir {
			return false
		}
		return files[i].Name < files[j].Name
	})

	// 准备模板数据
	data := struct {
		Path  string
		Files []FileInfo
	}{
		Path:  urlPath,
		Files: files,
	}

	// 解析并执行模板
	tmpl, err := template.New("dirlist").Parse(dirListTemplate)
	if err != nil {
		http.Error(w, "模板解析错误", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	err = tmpl.Execute(w, data)
	if err != nil {
		http.Error(w, "模板执行错误", http.StatusInternalServerError)
		return
	}
}
