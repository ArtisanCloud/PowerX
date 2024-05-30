package pluginx

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

const (
	pluginFrontendDir         = ".plugin"
	downloadDir               = ".download"
	archiveDir                = ".archive"
	pluginFrontendDownloadURL = "https://codeload.github.com/northseadl/PowerXDashboardPlugin/zip/refs/heads/master"
	pluginMasterDirName       = "PowerXDashboardPlugin-master"
)

var (
	pluginViewWhitelist = []string{"not-found", "redirect", "home"}
)

type Loader struct {
	workdir string
	cfg     *BuildLoaderConfig
}

// NewLoader 创建 Loader
func NewLoader(workdir string, cfg *BuildLoaderConfig) *Loader {
	return &Loader{
		workdir: workdir,
		cfg:     cfg,
	}
}

func (l *Loader) CheckEnvDependency() error {
	logger.Info("checking environment dependency")
	// 检查 yarn 是否安装
	_, err := exec.LookPath("yarn")
	if err != nil {
		return fmt.Errorf("yarn not found, please install yarn first")
	}

	// 检查 node 版本, 要求 >= 16
	nodeVersion, err := exec.Command("node", "-v").Output()
	if err != nil {
		return fmt.Errorf("node not found, please install node first")
	}
	if strings.Contains(string(nodeVersion), "v") {
		nodeVersion = nodeVersion[1:]
	}
	if strings.Compare(string(nodeVersion), "16") < 0 {
		return fmt.Errorf("node version must >= 16")
	}

	return nil
}

type BuildPluginFrontendOptions struct {
	ReDownload bool
}

// BuildPluginFrontend 构建插件前端
func (l *Loader) BuildPluginFrontend(opts BuildPluginFrontendOptions) error {
	// 检查是否workdir是否存在.download文件夹
	isFirstDownload := false
	downloadDir := filepath.Join(l.workdir, downloadDir)
	if _, err := os.Stat(downloadDir); os.IsNotExist(err) {
		logger.Info("creating download directory")
		err = os.MkdirAll(downloadDir, os.ModePerm)
		if err != nil {
			return err
		}
		isFirstDownload = true
	}

	// 如果已经存在, opts.ReDownload 为 true 则删除后重新创建
	if opts.ReDownload && !isFirstDownload {
		logger.Info("re-downloading plugin frontend")
		err := os.RemoveAll(downloadDir)
		if err != nil {
			return err
		}
	}

	// 如果是第一次下载或者重新下载, 则下载插件前端
	if isFirstDownload || opts.ReDownload {
		err := DownloadFile(pluginFrontendDownloadURL, downloadDir, "plugin.zip")
		if err != nil {
			return err
		}
	}

	// 删除 .plugin 目录
	pDir := filepath.Join(l.workdir, pluginFrontendDir)
	if _, err := os.Stat(pDir); !os.IsNotExist(err) {
		logger.Info("removing plugin directory")
		err = os.RemoveAll(pDir)
		if err != nil {
			return err
		}
	}

	// 解压插件前端
	err := UnzipFile(filepath.Join(downloadDir, "plugin.zip"), downloadDir)
	if err != nil {
		return err
	}

	// 复制 PowerXDashboardPlugin-master 为 .plugin
	err = CopyAndRenameDir(filepath.Join(downloadDir, pluginMasterDirName), pDir)

	// 清理插件前端目录下的多余文件
	err = cleanFrontend(pDir)
	if err != nil {
		return err
	}

	// 合并插件前端文件
	buildSomething, err := mergePluginFiles(l.workdir, pDir)
	if err != nil {
		return err
	}

	// 设置插件前端文件
	err = setupFrontendFiles(pDir, setupOptions{
		APIBaseURL: l.cfg.MainAPIEndpoint,
	})

	// 执行构建
	err = buildFrontend(pDir)
	if err != nil {
		return err
	}

	// 检查是否存在 .config 文件夹, 如果不存在则创建
	cacheDir := filepath.Join(l.workdir, ".config")
	if _, err := os.Stat(cacheDir); os.IsNotExist(err) {
		logger.Info("creating cache directory")
		err = os.MkdirAll(cacheDir, os.ModePerm)
		if err != nil {
			return err
		}
	}

	// 将插件信息写入 .config/build.yaml
	buildInfoFile, err := os.Create(filepath.Join(cacheDir, "build.yaml"))
	if err != nil {
		return err
	}
	defer buildInfoFile.Close()
	lockContent, err := yaml.Marshal(buildSomething.Build)
	if err != nil {
		return err
	}
	_, err = buildInfoFile.Write(lockContent)
	if err != nil {
		return err
	}

	etcFilePath := filepath.Join(cacheDir, "etc.yaml")
	etcFile, err := os.OpenFile(etcFilePath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return err
	}
	defer etcFile.Close()

	// 解析已存在的 etc.yaml
	etcFileContent, err := io.ReadAll(etcFile)
	if err != nil {
		return err
	}
	var etc PluginManagerEtc
	err = yaml.Unmarshal(etcFileContent, &etc)
	if err != nil {
		return err
	}

	// 比较已存在的 etc.yaml 和新的 etc.yaml, 如果已存在的 etc.yaml 中存在新的插件, 则使用已存在的配置
	for _, plugin := range buildSomething.Etc.Plugins {
		for _, etcPlugin := range etc.Plugins {
			if plugin.Name == etcPlugin.Name {
				plugin.Enable = etcPlugin.Enable
				plugin.Etc = etcPlugin.Etc
				// 检查map中的key是否一致, 不一致应该增加
				for key, value := range etcPlugin.Etc {
					if _, ok := plugin.Etc[key]; !ok {
						plugin.Etc[key] = value
					}
				}
			}
		}
	}

	etcContent, err := yaml.Marshal(buildSomething.Etc)
	if err != nil {
		return err
	}
	if err = etcFile.Truncate(0); err != nil {
		return err
	}
	if _, err = etcFile.Seek(0, 0); err != nil {
		return err
	}
	_, err = etcFile.Write(etcContent)

	// 将插件前端配置写入 .config/frontend.yaml
	frontendFile, err := os.Create(filepath.Join(cacheDir, "frontend.yaml"))
	if err != nil {
		return err
	}
	defer frontendFile.Close()
	frontendContent, err := yaml.Marshal(buildSomething.Frontend)
	if err != nil {
		return err
	}
	_, err = frontendFile.Write(frontendContent)
	if err != nil {
		return err
	}
	return nil
}

func cleanFrontend(dir string) error {
	logger.Info("cleaning frontend for building plugin")
	// 如果 src/router/routes/modules 目录存在, 则重新创建
	moduleDir := filepath.Join(dir, "src/router/routes/modules")
	if _, err := os.Stat(moduleDir); !os.IsNotExist(err) {
		logger.Info("removing src/router/routes/modules directory and re-creating")
		err = os.RemoveAll(moduleDir)
		if err != nil {
			return err
		}
		err = os.MkdirAll(moduleDir, os.ModePerm)
		if err != nil {
			return err
		}
	}

	// 清理 src/views 目录下除了白名单以外的所有目录
	viewDir := filepath.Join(dir, "src/views")
	if _, err := os.Stat(viewDir); !os.IsNotExist(err) {
		logger.Info("cleaning src/views directory")
		entries, err := os.ReadDir(viewDir)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if entry.IsDir() && !slices.Contains(pluginViewWhitelist, entry.Name()) {
				err = os.RemoveAll(filepath.Join(viewDir, entry.Name()))
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func mergePluginFiles(pluginDir string, frontDir string) (buildFiles *BuildData, err error) {
	var buildInfos []BuildPluginItem
	var pluginManagerConfig PluginManagerEtc
	var pluginFrontendConfig PluginFrontendInfo
	var mergedNames []string

	// 获取目录下的所有不以.开头的一级目录
	entries, err := os.ReadDir(pluginDir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		// 如果发现 mergedNames 中已经存在该插件名, 则报错
		if slices.Contains(mergedNames, entry.Name()) {
			return nil, fmt.Errorf("plugin %s already exists", entry.Name())
		}

		// 检查目录下是否存在 info.yaml, 如果不存在则跳过
		infoPath := filepath.Join(pluginDir, entry.Name(), "info.yaml")
		if _, err := os.Stat(infoPath); os.IsNotExist(err) {
			continue
		}

		logger.Info(fmt.Sprintf("merge plugin files: %s", entry.Name()))

		var info BuildPluginItem
		// 获取 info.yaml
		infoFile, err := os.ReadFile(infoPath)
		if err != nil {
			return nil, err
		}
		// 解析 info.yaml
		err = yaml.Unmarshal(infoFile, &info)
		if err != nil {
			return nil, err
		}
		// 将 info.Backend 转换为绝对路径
		info.Backend, err = filepath.Abs(filepath.Join(pluginDir, entry.Name(), info.Backend))
		if err != nil {
			return nil, err
		}

		// 解析目录下的 etc.yaml 文件, 如果不存在创建一个空的 etc 结构体
		etc := PluginEtc{
			Name: info.Name,
			// 默认启用, 如果合并时发现已存在该插件的配置, 则使用已存在的配置
			Enable: true,
			Etc:    make(PluginEtcMap),
		}
		etcPath := filepath.Join(pluginDir, entry.Name(), "etc.yaml")
		if _, err := os.Stat(etcPath); !os.IsNotExist(err) {
			etcFile, err := os.ReadFile(etcPath)
			if err != nil {
				return nil, err
			}
			err = yaml.Unmarshal(etcFile, &etc.Etc)
			fmt.Println(etc.Etc)
			if err != nil {
				return nil, err
			}
		}
		pluginManagerConfig.Plugins = append(pluginManagerConfig.Plugins, etc)

		// 解析目录下的 route.json 文件, 如果不存在返回错误
		routePath := filepath.Join(pluginDir, entry.Name(), "route.json")
		if _, err := os.Stat(routePath); os.IsNotExist(err) {
			return nil, fmt.Errorf("%s route.json not found", entry.Name())
		}
		routeFile, err := os.ReadFile(routePath)
		if err != nil {
			return nil, err
		}
		var route PluginFrontendRoute
		err = yaml.Unmarshal(routeFile, &route)
		if err != nil {
			return nil, err
		}
		pluginFrontendConfig.Routes = append(pluginFrontendConfig.Routes, route)

		fmt.Println(info.Name)

		// 复制 frontend/views 目录到 src/views/{插件名} 目录下
		err = CopyAndRenameDir(filepath.Join(pluginDir, entry.Name(), "frontend/views"), filepath.Join(frontDir, "src/views", info.Name))
		if err != nil {
			return nil, err
		}

		// 复制 frontend/module.ts 到 src/router/routes/modules/{插件名}.ts
		moduleFilePath := filepath.Join(frontDir, "src/router/routes/modules", info.Name+".ts")
		err = CopyAndRenameFile(filepath.Join(pluginDir, entry.Name(), "frontend/module.ts"), moduleFilePath)

		// 检查 frontend/api 目录下是否存在文件, 如果存在则复制到 src/api 目录下
		apiDir := filepath.Join(pluginDir, entry.Name(), "frontend", "api")
		if _, err := os.Stat(apiDir); !os.IsNotExist(err) {
			entries, err := os.ReadDir(apiDir)
			if err != nil {
				return nil, err
			}
			for _, entry := range entries {
				// 如果发现有不以 name 开头的文件, 则抛出错误
				lowerName := strings.ToLower(info.Name)
				if !strings.HasPrefix(entry.Name(), lowerName) {
					return nil, fmt.Errorf("plugin %s api file name must start with %s", info.Name, lowerName)
				}
				// 文件类型复制到 src/api 目录下
				if !entry.IsDir() {
					err = CopyAndRenameFile(filepath.Join(apiDir, entry.Name()), filepath.Join(frontDir, "src/api", entry.Name()))
					if err != nil {
						return nil, err
					}
				}
			}
		}

		// 复制 frontend/assets/images/* 到 src/assets/images
		imagesSrcDir := filepath.Join(pluginDir, entry.Name(), "frontend/assets/images")
		imagesDestDir := filepath.Join(frontDir, "src/assets/images")
		if _, err := os.Stat(imagesSrcDir); !os.IsNotExist(err) {
			err = CopyDir(imagesSrcDir, imagesDestDir)
			if err != nil {
				return nil, err
			}
		}

		// todo assert文件夹替换资源文件路径

		logger.Info(fmt.Sprintf("merge plugin files: %s  done", entry.Name()))

		// 将插件名存入 mergedNames
		mergedNames = append(mergedNames, entry.Name())
		// 将插件信息存入 buildInfos
		buildInfos = append(buildInfos, info)
	}
	return &BuildData{
		Build:    BuildInfo{Plugins: buildInfos, BuildDate: time.Now().Format("2006-01-02 15:04:05")},
		Etc:      pluginManagerConfig,
		Frontend: pluginFrontendConfig,
	}, nil
}

type setupOptions struct {
	APIBaseURL string
}

func setupFrontendFiles(dir string, opts setupOptions) error {
	// 写入 .env
	err := os.WriteFile(filepath.Join(dir, ".env"), []byte(fmt.Sprintf("VITE_API_BASE_URL=%s", opts.APIBaseURL)), os.ModePerm)
	if err != nil {
		return err
	}
	return nil
}

func buildFrontend(dir string) error {
	logger.Info("building plugin frontend")
	// 执行 yarn install
	cmd := exec.Command("yarn", "install")
	cmd.Dir = dir
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err := cmd.Run()
	if err != nil {
		return err
	}

	// 执行 yarn build
	cmd = exec.Command("yarn", "build")
	cmd.Dir = dir
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err = cmd.Run()
	if err != nil {
		return err
	}
	return nil
}

func (l *Loader) UnArchives() error {
	// 检查是否workdir是否存在.archive文件夹, 如果不存在则创建
	archiveDir := filepath.Join(l.workdir, archiveDir)
	if _, err := os.Stat(archiveDir); os.IsNotExist(err) {
		logger.Info("creating archive directory")
		err = os.MkdirAll(archiveDir, os.ModePerm)
		if err != nil {
			return err
		}
	}

	// 获取.archive目录下的所有.zip文件
	entries, err := os.ReadDir(archiveDir)
	if err != nil {
		return err
	}

	// 遍历所有.zip文件, 解压到 workdir 下
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".zip") {
			continue
		}
		logger.Info(fmt.Sprintf("unarchive plugin: %s", entry.Name()))
		fileNameWithoutExt := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		if err := UnzipFile(filepath.Join(archiveDir, entry.Name()), filepath.Join(l.workdir, strings.ToLower(fileNameWithoutExt))); err != nil {
			return err
		}
		logger.Info(fmt.Sprintf("unarchive plugin: %s done", entry.Name()))
	}
	return nil
}
