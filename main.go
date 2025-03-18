package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Config 环境配置
type Config struct {
	Environments map[string]Environment `json:"environments"`
}

// Environment 环境信息
type Environment struct {
	Context   string `json:"context"`
	Registry  string `json:"registry"`
	Namespace string `json:"namespace"`
}

// ServiceConfig 服务配置
type ServiceConfig struct {
	SingleServices map[string]ServiceInfo   `json:"single_services"`
	ServiceGroups  map[string]ServiceGroup  `json:"service_groups"`
}

// ServiceInfo 单个服务信息
type ServiceInfo struct {
	Enabled bool   `json:"enabled"`
	Version string `json:"version"`
}

// ServiceGroup 服务组信息
type ServiceGroup struct {
	Enabled  bool     `json:"enabled"`
	Version  string   `json:"version"`
	Services []string `json:"services"`
}

// Service 部署服务
type Service struct {
	config   Config
	services ServiceConfig
	logFile  string // 添加日志文件路径
}

// NewService 创建服务实例
func NewService() *Service {
	// 生成带时间戳的日志文件名
	timestamp := time.Now().Format("20060102_150405")
	logFile := filepath.Join(logDir, fmt.Sprintf("deploy_%s.log", timestamp))
	
	// 确保日志目录存在
	if err := os.MkdirAll(logDir, 0755); err != nil {
		fmt.Printf("创建日志目录失败: %v\n", err)
	}
	
	return &Service{
		logFile: logFile,
	}
}

// LoadConfig 加载配置文件
func LoadConfig(filename string, v interface{}) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

// LoadConfigs 加载配置
func (s *Service) LoadConfigs() error {
	if err := LoadConfig("config.json", &s.config); err != nil {
		return fmt.Errorf("读取 config.json 失败: %v", err)
	}

	if err := LoadConfig("services.json", &s.services); err != nil {
		return fmt.Errorf("读取 services.json 失败: %v", err)
	}

	return nil
}

// 定义颜色常量
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[36m"
)

// formatWithColor 使用指定颜色格式化字符串
func formatWithColor(color string, format string, args ...interface{}) string {
	return fmt.Sprintf(color+format+colorReset, args...)
}

// previewVersions 预览版本变更
func (s *Service) previewVersions(currentVersions, targetVersions map[string]string) {
	// 收集所有服务名称并排序
	var services []string
	for service := range targetVersions {
		services = append(services, service)
	}
	sort.Strings(services)

	// 计算最大宽度
	maxNameLen := len("服务名称")
	maxVersionLen := len("目标版本")
	
	for _, svc := range services {
		if len(svc) > maxNameLen {
			maxNameLen = len(svc)
		}
		currentVer := currentVersions[svc]
		targetVer := targetVersions[svc]
		if len(currentVer) > maxVersionLen {
			maxVersionLen = len(currentVer)
		}
		if len(targetVer) > maxVersionLen {
			maxVersionLen = len(targetVer)
		}
	}

	// 添加内边距
	maxNameLen += 2
	maxVersionLen += 2

	// 计算总宽度
	totalWidth := maxNameLen + (maxVersionLen * 2) + 7

	// 打印表头和分隔线
	s.printTableHeader(maxNameLen, maxVersionLen, totalWidth)

	// 打印服务信息
	format := fmt.Sprintf("%%s%%-%ds  %%s  ", maxNameLen)
	for _, svc := range services {
		currentVer := currentVersions[svc]
		targetVer := targetVersions[svc]
		prefix := formatWithColor(colorYellow, "•") + " "

		// 格式化版本显示
		currentVerFormatted := formatWithColor(colorCyan, "%-*s", maxVersionLen, currentVer)
		versionColor := colorGreen
		if currentVer != targetVer {
			versionColor = colorRed
		}
		versionDisplay := formatWithColor(versionColor, "%-*s", maxVersionLen, targetVer)

		fmt.Printf(format+"%s\n", prefix, svc, currentVerFormatted, versionDisplay)
	}
	fmt.Println(strings.Repeat("─", totalWidth))

	// 打印图例说明
	s.printLegend()
}

// printTableHeader 打印表格头部
func (s *Service) printTableHeader(maxNameLen, maxVersionLen, totalWidth int) {
	fmt.Println("\n版本变更预览:")
	fmt.Println(strings.Repeat("─", totalWidth))
	titleFormat := fmt.Sprintf("%%-%ds  %%-%ds  %%-%ds", maxNameLen, maxVersionLen, maxVersionLen)
	fmt.Printf(titleFormat+"\n", "服务名称", "当前版本", "目标版本")
	fmt.Println(strings.Repeat("─", totalWidth))
}

// printLegend 打印图例说明
func (s *Service) printLegend() {
	fmt.Println("\n版本说明:")
	fmt.Printf(colorGreen+"%-*s"+colorReset+" %s\n", 10, "绿色", "表示版本相同，无需更新")
	fmt.Printf(colorRed+"%-*s"+colorReset+" %s\n", 10, "红色", "表示版本将变更")
}

// getCurrentVersions 获取当前版本信息
func (s *Service) getCurrentVersions(namespace string, versions map[string]string) error {
	// 获取所有部署的镜像信息
	cmd := exec.Command("kubectl", "get", "deployment", "-n", namespace, "-o", "jsonpath={range .items[*]}{.metadata.name}{\"\\t\"}{.spec.template.spec.containers[0].image}{\"\\n\"}{end}")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("获取部署信息失败: %s", string(output))
	}

	// 获取启用的服务列表
	enabledServices := s.getEnabledServices()

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) == 2 {
			serviceName := parts[0]
			if _, ok := enabledServices[serviceName]; ok {
				imageParts := strings.Split(parts[1], ":")
				if len(imageParts) == 2 {
					versions[serviceName] = imageParts[1]
				}
			}
		}
	}

	return nil
}

// getEnabledServices 获取所有启用的服务
func (s *Service) getEnabledServices() map[string]bool {
	enabledServices := make(map[string]bool)
	
	// 添加启用的单个服务
	for service, info := range s.services.SingleServices {
		if info.Enabled {
			enabledServices[service] = true
		}
	}
	
	// 添加启用的服务组中的服务
	for _, group := range s.services.ServiceGroups {
		if group.Enabled {
			for _, service := range group.Services {
				enabledServices[service] = true
			}
		}
	}
	
	return enabledServices
}

// getSelectedServices 获取要发布的服务及版本号
func (s *Service) getSelectedServices(servicesInput, versionInput string) map[string]string {
	selectedServices := make(map[string]string)

	// 如果未指定服务，则使用配置文件中所有启用的服务
	if servicesInput == "" {
		// 添加单个服务
		for service, info := range s.services.SingleServices {
			if info.Enabled {
				selectedServices[service] = info.Version
			}
		}
		// 添加服务组
		for _, group := range s.services.ServiceGroups {
			if group.Enabled {
				for _, service := range group.Services {
					selectedServices[service] = group.Version
				}
			}
		}
		return selectedServices
	}

	// 处理用户指定的服务
	serviceList := strings.Split(servicesInput, ",")
	for _, service := range serviceList {
		// 处理单个服务
		if info, exists := s.services.SingleServices[service]; exists && info.Enabled {
			selectedServices[service] = s.selectVersion(versionInput, info.Version)
		}

		// 处理服务组
		if group, exists := s.services.ServiceGroups[service]; exists && group.Enabled {
			for _, subService := range group.Services {
				selectedServices[subService] = s.selectVersion(versionInput, group.Version)
			}
		}
	}

	return selectedServices
}

// selectVersion 选择版本号
func (s *Service) selectVersion(inputVersion, defaultVersion string) string {
	if inputVersion != "" {
		return inputVersion
	}
	return defaultVersion
}

// switchContext 切换 Kubernetes 环境
func (s *Service) switchContext(context string) error {
	// 获取当前上下文
	cmd := exec.Command("kubectl", "config", "current-context")
	output, err := cmd.CombinedOutput()
	if err == nil {
		currentContext := strings.TrimSpace(string(output))
		if currentContext == context {
			fmt.Printf("\n当前 Kubernetes 环境: %s\n", context)
			return nil // 如果当前上下文与目标上下文相同，则不需要切换
		}
	}

	fmt.Printf("\n🔄 切换 Kubernetes 环境: %s\n", context)
	
	cmd = exec.Command("kubectl", "config", "use-context", context)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("切换集群失败: %s", string(output))
	}

	// 等待集群连接就绪
	checkCmd := exec.Command("kubectl", "get", "nodes")
	if output, err := checkCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("集群连接检查失败: %s", string(output))
	}

	return nil
}

// confirmDeploy 等待用户确认是否继续部署
func (s *Service) confirmDeploy() bool {
	fmt.Print("\n是否确认部署? [y/N]: ")
	var response string
	fmt.Scanln(&response)
	return strings.ToLower(response) == "y"
}

// 定义日志相关常量
const (
	logDir = "logs"
)

// writeLog 写入日志
func (s *Service) writeLog(format string, args ...interface{}) {
	// 打开日志文件（追加模式）
	f, err := os.OpenFile(s.logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf("打开日志文件失败: %v\n", err)
		return
	}
	defer f.Close()

	// 获取当前时间
	now := time.Now().Format("2006-01-02 15:04:05")
	
	// 格式化日志内容
	logContent := fmt.Sprintf("[%s] %s\n", now, fmt.Sprintf(format, args...))
	
	// 写入日志
	if _, err := f.WriteString(logContent); err != nil {
		fmt.Printf("写入日志失败: %v\n", err)
	}
}

// Deploy 部署服务
func (s *Service) Deploy(env, servicesInput, versionInput string) error {
	s.writeLog("开始部署操作 - 环境: %s, 服务: %s, 版本: %s", env, servicesInput, versionInput)

	// 获取环境配置
	envConfig, exists := s.config.Environments[env]
	if !exists {
		s.writeLog("错误: 无效的环境: %s", env)
		return fmt.Errorf("无效的环境: %s", env)
	}

	// 切换环境
	if err := s.switchContext(envConfig.Context); err != nil {
		s.writeLog("错误: 切换环境失败: %v", err)
		return err
	}
	s.writeLog("成功切换到环境: %s", envConfig.Context)

	// 获取当前版本信息
	currentVersions := make(map[string]string)
	if err := s.getCurrentVersions(envConfig.Namespace, currentVersions); err != nil {
		s.writeLog("警告: 获取当前版本信息失败: %v", err)
		fmt.Printf("获取当前版本信息失败: %v\n", err)
	}

	// 解析要部署的服务
	selectedServices := s.getSelectedServices(servicesInput, versionInput)
	if len(selectedServices) == 0 {
		s.writeLog("错误: 没有可用的服务进行发布")
		return fmt.Errorf("没有可用的服务进行发布")
	}
	s.writeLog("选中的服务: %v", selectedServices)

	// 打印版本对比预览
	s.previewVersions(currentVersions, selectedServices)

	// 等待用户确认
	if !s.confirmDeploy() {
		s.writeLog("用户取消部署")
		fmt.Println("\n❌ 已取消部署")
		return nil
	}

	s.writeLog("用户确认部署，开始执行...")
	fmt.Println("\n开始部署...")

	// 并行部署
	var wg sync.WaitGroup
	var mu sync.Mutex
	deployResults := make([]string, 0)

	for service, ver := range selectedServices {
		wg.Add(1)
		go func(service, ver string) {
			defer wg.Done()
			result := s.deployService(service, envConfig.Registry, envConfig.Namespace, ver)
			mu.Lock()
			deployResults = append(deployResults, result)
			s.writeLog("部署结果: %s", result)
			mu.Unlock()
		}(service, ver)
	}
	wg.Wait()

	// 按顺序打印部署结果
	sort.Strings(deployResults)
	for _, result := range deployResults {
		fmt.Println(result)
	}

	// 获取更新后的版本信息并打印对比
	updatedVersions := make(map[string]string)
	if err := s.getCurrentVersions(envConfig.Namespace, updatedVersions); err != nil {
		s.writeLog("警告: 获取更新后版本信息失败: %v", err)
		fmt.Printf("获取更新后版本信息失败: %v\n", err)
	}

	// 打印版本对比
	s.printVersionComparison(currentVersions, updatedVersions)
	s.writeLog("部署操作完成")

	return nil
}

// Check 检查版本差异但不执行部署
func (s *Service) Check(env, servicesInput, versionInput string) error {
	// 获取环境配置
	envConfig, exists := s.config.Environments[env]
	if !exists {
		return fmt.Errorf("无效的环境: %s", env)
	}

	// 切换环境
	if err := s.switchContext(envConfig.Context); err != nil {
		return err
	}

	// 获取当前版本信息
	currentVersions := make(map[string]string)
	if err := s.getCurrentVersions(envConfig.Namespace, currentVersions); err != nil {
		fmt.Printf("获取当前版本信息失败: %v\n", err)
	}

	// 解析要检查的服务
	selectedServices := s.getSelectedServices(servicesInput, versionInput)
	if len(selectedServices) == 0 {
		return fmt.Errorf("没有可用的服务进行检查")
	}

	// 打印版本对比预览
	s.previewVersions(currentVersions, selectedServices)

	return nil
}

// deployService 部署服务
func (s *Service) deployService(service, registry, namespace, version string) string {
	// 构建完整的镜像地址
	image := fmt.Sprintf("%s/%s:%s", registry, service, version)
	s.writeLog("开始发布服务 %s -> %s", service, image)
	fmt.Printf("🚀 发布 %s -> %s\n", service, image)

	cmd := exec.Command("kubectl", "set", "image", "deployment", service, 
		fmt.Sprintf("%s=%s", service, image), "-n", namespace)
	
	if output, err := cmd.CombinedOutput(); err != nil {
		errMsg := string(output)
		errMsg = strings.ReplaceAll(errMsg, "exit status 1", "")
		result := fmt.Sprintf("❌ %s 发布失败: %s", service, strings.TrimSpace(errMsg))
		s.writeLog(result)
		return result
	}
	
	result := fmt.Sprintf("✅ %s 发布成功", service)
	s.writeLog(result)
	return result
}

// printVersionComparison 打印版本对比
func (s *Service) printVersionComparison(currentVersions, updatedVersions map[string]string) {
	// 收集所有服务名称并排序
	var services []string
	for service := range updatedVersions {
		services = append(services, service)
	}
	sort.Strings(services)

	// 计算最大宽度
	maxNameLen := len("服务名称")
	maxVersionLen := len("之前版本")
	
	for _, svc := range services {
		if len(svc) > maxNameLen {
			maxNameLen = len(svc)
		}
		currentVer := currentVersions[svc]
		updatedVer := updatedVersions[svc]
		if len(currentVer) > maxVersionLen {
			maxVersionLen = len(currentVer)
		}
		if len(updatedVer) > maxVersionLen {
			maxVersionLen = len(updatedVer)
		}
	}

	// 添加内边距
	maxNameLen += 2
	maxVersionLen += 2

	// 计算总宽度
	totalWidth := maxNameLen + (maxVersionLen * 2) + 7

	// 打印表头
	fmt.Println("\n版本变更结果:")
	fmt.Println(strings.Repeat("─", totalWidth))
	titleFormat := fmt.Sprintf("%%-%ds  %%-%ds  %%-%ds", maxNameLen, maxVersionLen, maxVersionLen)
	fmt.Printf(titleFormat+"\n", "服务名称", "之前版本", "更新后版本")
	fmt.Println(strings.Repeat("─", totalWidth))

	// 打印服务信息
	format := fmt.Sprintf("%%s%%-%ds  %%s  ", maxNameLen)
	for _, svc := range services {
		currentVer := currentVersions[svc]
		updatedVer := updatedVersions[svc]
		prefix := formatWithColor(colorYellow, "•") + " "

		// 格式化版本显示
		currentVerFormatted := formatWithColor(colorCyan, "%-*s", maxVersionLen, currentVer)
		versionColor := colorGreen
		if currentVer != updatedVer {
			versionColor = colorRed
		}
		versionDisplay := formatWithColor(versionColor, "%-*s", maxVersionLen, updatedVer)

		fmt.Printf(format+"%s\n", prefix, svc, currentVerFormatted, versionDisplay)
	}
	fmt.Println(strings.Repeat("─", totalWidth))

	// 打印图例说明
	s.printLegend()
}

func main() {
	if len(os.Args) < 2 {
		showHelp()
		os.Exit(1)
	}

	// 检查第一个参数是否为 check
	isCheck := os.Args[1] == "check"
	var args []string
	if isCheck {
		args = os.Args[2:] // 如果是 check 命令，跳过 "check" 参数
	} else {
		args = os.Args[1:] // 否则使用所有参数
	}

	// 创建一个新的 FlagSet
	cmdFlags := flag.NewFlagSet("cmd", flag.ExitOnError)
	env := cmdFlags.String("env", "", "要操作的环境 (pre/prod)")
	services := cmdFlags.String("services", "", "要操作的服务, 逗号分隔 (web-fe,backend)")
	version := cmdFlags.String("version", "", "要操作的版本号")
	help := cmdFlags.Bool("help", false, "显示帮助信息")

	// 解析命令行参数
	cmdFlags.Parse(args)

	if *help {
		showHelp()
		return
	}

	if *env == "" {
		fmt.Println("❌ 需要指定环境: -env=<pre/prod>")
		os.Exit(1)
	}

	// 创建服务实例
	service := NewService()
	
	// 加载配置
	if err := service.LoadConfigs(); err != nil {
		fmt.Printf("❌ %v\n", err)
		os.Exit(1)
	}

	// 根据命令执行相应的操作
	var err error
	if isCheck {
		err = service.Check(*env, *services, *version)
	} else {
		err = service.Deploy(*env, *services, *version)
	}

	if err != nil {
		fmt.Printf("❌ %v\n", err)
		os.Exit(1)
	}
}

func showHelp() {
	fmt.Println("用法: go run main.go [check] [参数]")
	fmt.Println("\n命令:")
	fmt.Println("  check   检查版本变更（不执行部署）")
	fmt.Println("  (无)    直接执行部署")
	fmt.Println("\n参数:")
	fmt.Println("  -env string      要操作的环境 (pre/prod)")
	fmt.Println("  -services string 要操作的服务, 逗号分隔 (web-fe,backend)")
	fmt.Println("  -version string  要操作的版本号 (如果不指定，则使用默认 services.json)")
	fmt.Println("  -help           显示帮助信息")
	fmt.Println("\n示例:")
	fmt.Println("  go run main.go -env=pre")
	fmt.Println("  go run main.go -env=pre -services=web-fe -version=v1.0.0")
	fmt.Println("  go run main.go check -env=pre")
	fmt.Println("  go run main.go check -env=pre -services=web-fe")
}
