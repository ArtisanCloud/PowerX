package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func main() {
	// 创建Gin路由器
	r := gin.Default()

	// 定义Hello World路由
	r.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message": "Hello World!",
			"status":  "success",
		})
	})

	// 启动服务器，监听8080端口
	r.Run(":8080")
}