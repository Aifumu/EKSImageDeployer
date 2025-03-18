# EKS Image Deployer
# 📌 项目介绍
```
本项目用于在 AWS EKS 集群中自动部署Deplopment应用
支持多环境，例如（pre/prod）
支持自动切换 Kubernetes 上下文
支持指定服务部署
支持services.json 多服务进行自动化部署
```


# 🔧 安装配置
## 1️. 安装依赖
```
go mod init deployer

go mod tidy
```

## 2️. 配置 AWS 凭证（权限）
```
aws configure
```

## 3️. 修改json文件
```
{
  "environments": {
    "staging": {
      "context": "arn:aws:eks:us-west-2:123456789012:cluster/staging-cluster",
      "namespace": "default",
      "registry": "123456789012.dkr.ecr.us-west-2.amazonaws.com"
    },
    "prod": {
      "context": "arn:aws:eks:us-west-2:123456789012:cluster/prod-cluster",
      "namespace": "default",
      "registry": "123456789012.dkr.ecr.us-west-2.amazonaws.com"
    }
  }
}
```
## 4️. 增加要部署的服务
```
{
  "single_services": {
    "docs-fe": {
      "version": "v3.48.2",
      "enabled": true
    }
  },
  "service_groups": {
    "nft": {
      "version": "v1.15.5",
      "enabled": true,
      "services": [
        "nft-berachain-be",
        "nft-core-be",
        "nft-ethereum-be"
      ]
    },
    "test": {
      "version": "v1.15.5",
      "enabled": true,
      "services": [
        "web-test1-be",
        "web-test2-be",
        "web-test3-be"
      ]
    }
  }
}
```
## 5. 运行部署
```
# 帮助
go run main.go  --help

# 默认更新services.json里为true的服务
go run main.go -env=pre

# 检查当前要更新的版本并显示当前页面
go run main.go check -env=pre

# 检查指定服务的版本和要更新的版本
go run main.go  check -services docs-fe -env=pre

# 更新指定服务指定版本
go run main.go  -env=pre  -services=docs-fe -version=v3.48.2
```


# 📦 目录结构
```
EKSImageDeployer/
│── main.go             # 主程序入口
│── config.json         # 环境配置文件
│── services.json       # 服务配置文件
│── internal/logger/logger.go    # 日志管理模块
```

# 发布效果
![image](https://github.com/user-attachments/assets/33e21ce2-5f20-4960-88b7-ff7763984445)




