# EKSImageDeployer
🚀 一个用于 AWS EKS Kubernetes 部署的 Go 语言服务，支持自动部署镜像到 K8s 集群（注意：只支持deplopment）。
📌 项目介绍
本项目用于在 AWS EKS 集群中自动部署应用，提供以下功能：

支持多环境（staging/prod）
并行部署多个服务
自动切换 Kubernetes 上下文
读取 services.json 进行自动化管理

📦 目录结构
bash
复制
编辑
EKSImageDeployer/
│── config.json         # 环境配置文件
│── services.json       # 服务配置文件
│── internal/logger/    # 日志管理模块
│── main.go             # 主程序入口
│── README.md           # 项目说明文档
│── go.mod              # Go 依赖管理
