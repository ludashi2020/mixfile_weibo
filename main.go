package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"

	"github.com/gin-gonic/gin"
)

// Config 定义配置文件的结构
type Config struct {
	Cookie    string `json:"cookie"`
	ImagePath string `json:"image_path"`
	Referer   string `json:"referer"`
	Port      int    `json:"port"`
}

func loadConfig(filename string) (Config, error) {
	var config Config
	file, err := os.Open(filename)
	if err != nil {
		return config, err
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	err = decoder.Decode(&config)
	return config, err
}

func crc32(buffer []byte) uint32 {
	crc := uint32(0xffffffff)
	for _, b := range buffer {
		crc ^= uint32(b)
		for j := 0; j < 8; j++ {
			crc = (crc >> 1) ^ (-(crc & 1) & 0xedb88320)
		}
	}
	return (crc ^ 0xffffffff)
}

func main() {
	// 获取当前目录
	_, filename, _, _ := runtime.Caller(0)
	dir := filepath.Dir(filename)

	// 加载配置文件
	config, err := loadConfig(filepath.Join(dir, "config.json"))
	if err != nil {
		log.Fatalf("无法加载配置文件: %v", err)
	}

	// 初始化 Gin 路由器
	r := gin.Default()

	// 设置中间件
	r.Use(gin.Logger())
	r.Use(gin.Recovery())

	// PUT 端点
	r.PUT("/", func(c *gin.Context) {
		// 读取请求体
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.String(http.StatusInternalServerError, "服务器内部错误")
			return
		}

		// 准备请求到 Weibo API
		url := "https://picupload.weibo.com/interface/upload.php"
		req, err := http.NewRequest("POST", url, bytes.NewReader(body))
		if err != nil {
			c.String(http.StatusInternalServerError, "服务器内部错误")
			return
		}

		// 设置查询参数
		q := req.URL.Query()
		q.Add("cs", fmt.Sprintf("%d", crc32(body)))
		q.Add("ent", "miniblog")
		req.URL.RawQuery = q.Encode()

		// 设置请求头
		req.Header.Set("Content-Type", "image/gif")
		req.Header.Set("Cookie", config.Cookie) // 从配置文件读取 Cookie

		// 发送请求
		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("上传错误: %v", err)
			c.String(http.StatusInternalServerError, "服务器内部错误")
			return
		}
		defer resp.Body.Close()

		// 读取响应
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			c.String(http.StatusInternalServerError, "服务器内部错误")
			return
		}

		// 解析 JSON 响应
		var result struct {
			Pic struct {
				Pid string `json:"pid"`
			} `json:"pic"`
		}
		if err := json.Unmarshal(respBody, &result); err != nil || result.Pic.Pid == "" {
			c.String(http.StatusForbidden, "上传微博失败了")
			return
		}

		// 修改这里，使用 = 而不是 :=
		url = fmt.Sprintf("https://wx3.sinaimg.cn/large/%s", result.Pic.Pid)
		log.Printf("上传成功 %s", url)
		c.String(http.StatusOK, url)
	})

	// GET 端点
	r.GET("/", func(c *gin.Context) {
		filePath := filepath.Join(dir, config.ImagePath) // 从配置文件读取图片路径
		
		// 设置 referer 头
		c.Header("Referer", config.Referer) // 从配置文件读取 referer
		
		// 发送文件
		c.File(filePath)
	})

	// 启动服务器，使用配置文件中的端口
	addr := fmt.Sprintf(":%d", config.Port) // 从配置文件读取端口
	if err := r.Run(addr); err != nil {
		log.Fatal("服务器启动失败: ", err)
	}
	fmt.Printf("服务器运行在端口 %d\n", config.Port)
}
