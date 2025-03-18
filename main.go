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

// 定义常量
const (
	// 日志相关
	logDir = "logs"

	// 配置文件
	configFile   = "config.json"
	servicesFile = "services.json"

	// 颜色
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[36m"
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
	SingleServices map[string]ServiceInfo  `json:"single_services"`
	ServiceGroups  map[string]ServiceGroup `json:"service_groups"`
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
	logFile  string
	logger   *Logger
}

// Logger 日志管理器
type Logger struct {
	file string
}

// NewLogger 创建日志管理器
func NewLogger(logFile string) (*Logger, error) {
	if err := os.MkdirAll(filepath.Dir(logFile), 0755); err != nil {
		return nil, fmt.Errorf("创建日志目录失败: %v", err)
	}
	return &Logger{file: logFile}, nil
}

// Log 记录日志
func (l *Logger) Log(format string, args ...interface{}) error {
	f, err := os.OpenFile(l.file, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("打开日志文件失败: %v", err)
	}
	defer f.Close()

	now := time.Now().Format("2006-01-02 15:04:05")
	logContent := fmt.Sprintf("[%s] %s\n", now, fmt.Sprintf(format, args...))

	if _, err := f.WriteString(logContent); err != nil {
		return fmt.Errorf("写入日志失败: %v", err)
	}
	return nil
}

// NewService 创建服务实例
func NewService() (*Service, error) {
	timestamp := time.Now().Format("20060102_150405")
	logFile := filepath.Join(logDir, fmt.Sprintf("deploy_%s.log", timestamp))

	logger, err := NewLogger(logFile)
	if err != nil {
		return nil, err
	}

	return &Service{
		logFile: logFile,
		logger:  logger,
	}, nil
}

// LoadConfig 加载配置文件
func LoadConfig(filename string, v interface{}) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("读取配置文件 %s 失败: %v", filename, err)
	}
	return json.Unmarshal(data, v)
}

// Init 初始化服务
func (s *Service) Init() error {
	// 加载配置文件
	if err := LoadConfig(configFile, &s.config); err != nil {
		return err
	}

	if err := LoadConfig(servicesFile, &s.services); err != nil {
		return err
	}

	return nil
}

// formatWithColor 使用指定颜色格式化字符串
func formatWithColor(color, format string, args ...interface{}) string {
	return fmt.Sprintf(color+format+colorReset, args...)
}

// getCurrentVersions 获取当前版本信息
func (s *Service) getCurrentVersions(namespace string) (map[string]string, error) {
	versions := make(map[string]string)
	cmd := exec.Command("kubectl", "get", "deployment", "-n", namespace, "-o", "jsonpath={range .items[*]}{.metadata.name}{\"\\t\"}{.spec.template.spec.containers[0].image}{\"\\n\"}{end}")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("获取部署信息失败: %s", string(output))
	}

	enabledServices := s.getEnabledServices()
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) != 2 {
			continue
		}

		serviceName := parts[0]
		if _, ok := enabledServices[serviceName]; !ok {
			continue
		}

		imageParts := strings.Split(parts[1], ":")
		if len(imageParts) == 2 {
			versions[serviceName] = imageParts[1]
		}
	}

	return versions, nil
}

// getEnabledServices 获取所有启用的服务
func (s *Service) getEnabledServices() map[string]bool {
	enabledServices := make(map[string]bool)

	for service, info := range s.services.SingleServices {
		if info.Enabled {
			enabledServices[service] = true
		}
	}

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

	if servicesInput == "" {
		// 添加所有启用的服务
		for service, info := range s.services.SingleServices {
			if info.Enabled {
				selectedServices[service] = s.selectVersion(versionInput, info.Version)
			}
		}

		for _, group := range s.services.ServiceGroups {
			if group.Enabled {
				version := s.selectVersion(versionInput, group.Version)
				for _, service := range group.Services {
					selectedServices[service] = version
				}
			}
		}
		return selectedServices
	}

	// 处理指定的服务
	servicesList := strings.Split(servicesInput, ",")
	for _, service := range servicesList {
		service = strings.TrimSpace(service)

		// 检查单个服务
		if info, ok := s.services.SingleServices[service]; ok && info.Enabled {
			selectedServices[service] = s.selectVersion(versionInput, info.Version)
			continue
		}

		// 检查服务组
		if group, ok := s.services.ServiceGroups[service]; ok && group.Enabled {
			version := s.selectVersion(versionInput, group.Version)
			for _, groupService := range group.Services {
				selectedServices[groupService] = version
			}
			continue
		}

		s.logger.Log("警告: 服务 %s 未找到或未启用", service)
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

// switchContext 切换环境上下文
func (s *Service) switchContext(context string) error {
	cmd := exec.Command("kubectl", "config", "use-context", context)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("切换上下文失败: %s", string(output))
	}
	return nil
}

// confirmDeploy 确认部署
func (s *Service) confirmDeploy() bool {
	fmt.Print("\n是否确认部署? [y/N]: ")
	var response string
	fmt.Scanln(&response)
	return strings.ToLower(response) == "y"
}

// deployService 部署单个服务
func (s *Service) deployService(service, registry, namespace, version string) error {
	image := fmt.Sprintf("%s/%s:%s", registry, service, version)
	s.logger.Log("开始发布服务 %s -> %s", service, image)
	fmt.Printf("🚀 发布 %s -> %s\n", service, image)

	cmd := exec.Command("kubectl", "set", "image", "deployment", service,
		fmt.Sprintf("%s=%s", service, image), "-n", namespace)

	if output, err := cmd.CombinedOutput(); err != nil {
		errMsg := strings.ReplaceAll(string(output), "exit status 1", "")
		s.logger.Log("服务 %s 发布失败: %s", service, errMsg)
		return fmt.Errorf("❌ %s 发布失败: %s", service, strings.TrimSpace(errMsg))
	}

	s.logger.Log("服务 %s 发布成功", service)
	return nil
}

// Deploy 部署服务
func (s *Service) Deploy(env, servicesInput, versionInput string) error {
	s.logger.Log("开始部署操作 - 环境: %s, 服务: %s, 版本: %s", env, servicesInput, versionInput)

	// 获取环境配置
	envConfig, exists := s.config.Environments[env]
	if !exists {
		return fmt.Errorf("无效的环境: %s", env)
	}

	// 切换环境
	if err := s.switchContext(envConfig.Context); err != nil {
		return err
	}
	s.logger.Log("成功切换到环境: %s", envConfig.Context)

	// 获取当前版本信息
	currentVersions, err := s.getCurrentVersions(envConfig.Namespace)
	if err != nil {
		s.logger.Log("警告: %v", err)
		fmt.Printf("警告: %v\n", err)
	}

	// 获取要部署的服务
	selectedServices := s.getSelectedServices(servicesInput, versionInput)
	if len(selectedServices) == 0 {
		return fmt.Errorf("没有可用的服务进行发布")
	}
	s.logger.Log("选中的服务: %v", selectedServices)

	// 打印版本对比预览
	s.previewVersions(currentVersions, selectedServices)

	// 等待用户确认
	if !s.confirmDeploy() {
		s.logger.Log("用户取消部署")
		fmt.Println("\n❌ 已取消部署")
		return nil
	}

	s.logger.Log("用户确认部署，开始执行...")
	fmt.Println("\n开始部署...")

	// 并行部署
	var wg sync.WaitGroup
	errChan := make(chan error, len(selectedServices))

	for service, version := range selectedServices {
		wg.Add(1)
		go func(service, version string) {
			defer wg.Done()
			if err := s.deployService(service, envConfig.Registry, envConfig.Namespace, version); err != nil {
				errChan <- err
			}
		}(service, version)
	}

	wg.Wait()
	close(errChan)

	// 收集错误
	var errors []string
	for err := range errChan {
		errors = append(errors, err.Error())
	}

	if len(errors) > 0 {
		return fmt.Errorf("部署过程中出现错误:\n%s", strings.Join(errors, "\n"))
	}

	// 获取更新后的版本信息
	updatedVersions, err := s.getCurrentVersions(envConfig.Namespace)
	if err != nil {
		s.logger.Log("警告: 获取更新后版本信息失败: %v", err)
		fmt.Printf("警告: 获取更新后版本信息失败: %v\n", err)
	}

	// 打印版本对比
	s.printVersionComparison(currentVersions, updatedVersions)
	s.logger.Log("部署操作完成")

	return nil
}

// Check 检查版本
func (s *Service) Check(env, servicesInput, versionInput string) error {
	envConfig, exists := s.config.Environments[env]
	if !exists {
		return fmt.Errorf("无效的环境: %s", env)
	}

	if err := s.switchContext(envConfig.Context); err != nil {
		return err
	}

	currentVersions, err := s.getCurrentVersions(envConfig.Namespace)
	if err != nil {
		return err
	}

	selectedServices := s.getSelectedServices(servicesInput, versionInput)
	if len(selectedServices) == 0 {
		return fmt.Errorf("没有可用的服务进行检查")
	}

	s.previewVersions(currentVersions, selectedServices)
	return nil
}

// previewVersions 预览版本变更
func (s *Service) previewVersions(currentVersions, targetVersions map[string]string) {
	var services []string
	for service := range targetVersions {
		services = append(services, service)
	}
	sort.Strings(services)

	maxNameLen := len("服务名称")
	maxVersionLen := len("目标版本")

	for _, svc := range services {
		if len(svc) > maxNameLen {
			maxNameLen = len(svc)
		}
		if len(currentVersions[svc]) > maxVersionLen {
			maxVersionLen = len(currentVersions[svc])
		}
		if len(targetVersions[svc]) > maxVersionLen {
			maxVersionLen = len(targetVersions[svc])
		}
	}

	maxNameLen += 2
	maxVersionLen += 2
	totalWidth := maxNameLen + (maxVersionLen * 2) + 7

	s.printTableHeader(maxNameLen, maxVersionLen, totalWidth)
	s.printVersionRows(services, currentVersions, targetVersions, maxNameLen, maxVersionLen)
	fmt.Println(strings.Repeat("─", totalWidth))
	s.printLegend(true)
}

// printTableHeader 打印表格头部
func (s *Service) printTableHeader(maxNameLen, maxVersionLen, totalWidth int) {
	fmt.Println("\n版本变更预览:")
	fmt.Println(strings.Repeat("─", totalWidth))
	titleFormat := fmt.Sprintf("%%-%ds  %%-%ds  %%-%ds", maxNameLen, maxVersionLen, maxVersionLen)
	fmt.Printf(titleFormat+"\n", "服务名称", "当前版本", "目标版本")
	fmt.Println(strings.Repeat("─", totalWidth))
}

// printVersionRows 打印版本行
func (s *Service) printVersionRows(services []string, currentVersions, targetVersions map[string]string, maxNameLen, maxVersionLen int) {
	format := fmt.Sprintf("%%s%%-%ds  %%s  ", maxNameLen)
	for _, svc := range services {
		currentVer := currentVersions[svc]
		targetVer := targetVersions[svc]
		prefix := formatWithColor(colorYellow, "•") + " "

		currentVerFormatted := formatWithColor(colorCyan, "%-*s", maxVersionLen, currentVer)
		versionColor := colorGreen
		if currentVer != targetVer {
			versionColor = colorRed
		}
		versionDisplay := formatWithColor(versionColor, "%-*s", maxVersionLen, targetVer)

		fmt.Printf(format+"%s\n", prefix, svc, currentVerFormatted, versionDisplay)
	}
}

// printLegend 打印图例说明
func (s *Service) printLegend(isPreview bool) {
	fmt.Println("\n版本说明:")
	fmt.Printf(colorGreen+"%-*s"+colorReset+" %s\n", 10, "绿色", "表示版本相同，无需更新")
	if isPreview {
		fmt.Printf(colorRed+"%-*s"+colorReset+" %s\n", 10, "红色", "表示版本将变更")
	} else {
		fmt.Printf(colorRed+"%-*s"+colorReset+" %s\n", 10, "红色", "表示版本已变更")
	}
}

// printVersionComparison 打印版本对比
func (s *Service) printVersionComparison(oldVersions, newVersions map[string]string) {
	fmt.Println("\n部署结果:")
	var services []string
	for service := range newVersions {
		services = append(services, service)
	}
	sort.Strings(services)

	maxNameLen := len("服务名称")
	maxVersionLen := len("目标版本")

	for _, svc := range services {
		if len(svc) > maxNameLen {
			maxNameLen = len(svc)
		}
		if len(oldVersions[svc]) > maxVersionLen {
			maxVersionLen = len(oldVersions[svc])
		}
		if len(newVersions[svc]) > maxVersionLen {
			maxVersionLen = len(newVersions[svc])
		}
	}

	maxNameLen += 2
	maxVersionLen += 2
	totalWidth := maxNameLen + (maxVersionLen * 2) + 7

	fmt.Println(strings.Repeat("─", totalWidth))
	s.printVersionRows(services, oldVersions, newVersions, maxNameLen, maxVersionLen)
	fmt.Println(strings.Repeat("─", totalWidth))
	s.printLegend(false)
}

func main() {
	// 创建一个新的 FlagSet
	cmdFlags := flag.NewFlagSet("cmd", flag.ExitOnError)
	env := cmdFlags.String("env", "", "部署环境 (必填)")
	services := cmdFlags.String("services", "", "要部署的服务，多个服务用逗号分隔")
	version := cmdFlags.String("version", "", "指定版本号")
	help := cmdFlags.Bool("help", false, "显示帮助信息")

	// 检查是否为 check 命令
	isCheck := len(os.Args) > 1 && os.Args[1] == "check"
	var args []string
	if isCheck {
		args = os.Args[2:] // 跳过 "check" 参数
	} else {
		args = os.Args[1:] // 使用所有参数
	}

	// 解析命令行参数
	if err := cmdFlags.Parse(args); err != nil {
		fmt.Printf("解析参数失败: %v\n", err)
		showHelp()
		os.Exit(1)
	}

	// 显示帮助信息
	if *help {
		showHelp()
		return
	}

	// 检查必填参数
	if *env == "" {
		fmt.Println("错误: 必须指定部署环境")
		showHelp()
		os.Exit(1)
	}

	// 创建服务实例
	service, err := NewService()
	if err != nil {
		fmt.Printf("创建服务实例失败: %v\n", err)
		os.Exit(1)
	}

	// 初始化服务
	if err := service.Init(); err != nil {
		fmt.Printf("初始化服务失败: %v\n", err)
		os.Exit(1)
	}

	// 执行操作
	var operationErr error
	if isCheck {
		operationErr = service.Check(*env, *services, *version)
	} else {
		operationErr = service.Deploy(*env, *services, *version)
	}

	if operationErr != nil {
		fmt.Printf("操作失败: %v\n", operationErr)
		os.Exit(1)
	}
}

func showHelp() {
	helpText := `
使用说明:
  go run main.go [check] [选项]

命令:
  check         检查版本变更（不执行部署）
  (无)          直接执行部署

必填选项:
  -env string
        部署环境 (例如: dev, pre, prod)

可选选项:
  -services string
        要部署的服务，多个服务用逗号分隔
        不指定则部署所有已启用的服务
  -version string
        指定版本号
        不指定则使用配置文件中的版本号
  -help
        显示帮助信息

示例:
  # 部署所有服务
  go run main.go -env=pre

  # 部署指定服务
  go run main.go -env=pre -services=docs-fe -version=v3.48.2

  # 检查所有服务版本
  go run main.go check -env=pre

  # 检查指定服务版本
  go run main.go check -env=pre -services=docs-fe
`
	fmt.Println(helpText)
}
