package profile

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"

	"uamemos/service/version"
)

// 定义配置的字段
type Profile struct {
	Mode    string `json:"mode"`
	Port    int    `json:"-"`
	Data    string `json:"-"` // 数据目录
	DSN     string `json:"-"` // 数据源名称
	Version string `json:"version"`
}

// func (p *Profile) isDev() bool {
// 	return p.Mode != "prod"
// }

func checkDSN(dataDir string) (string, error) {
	if !filepath.IsAbs(dataDir) {
		// 第一个参数为程序运行目录
		absDir, err := filepath.Abs(filepath.Dir(os.Args[0]) + "/" + dataDir)
		fmt.Printf("abs dir: %s-%s\n", dataDir, absDir)
		if err != nil {
			return "", err
		}
		dataDir = absDir
	}
	// 判断文件是否存在，不存在则返回错误
	dataDir = strings.TrimRight(dataDir, "/")
	if _, err := os.Stat(dataDir); err != nil {
		return "", fmt.Errorf("unable to access data folder %s, err %w", dataDir, err)
	}
	return dataDir, nil
}

func GetProfile() (*Profile, error) {
	profile := Profile{}
	// 读取配置
	if err := viper.Unmarshal(&profile); err != nil {
		return nil, err
	}
	if profile.Mode != "demo" && profile.Mode != "dev" && profile.Mode != "prod" {
		profile.Mode = "demo"
	}
	if profile.Mode == "prod" && profile.Data == "" {
		profile.Data = "./data"
	}
	dataDir, err := checkDSN(profile.Data)
	if err != nil {
		fmt.Printf("Failed to check dsn: %s, err: %+v\n", dataDir, err)
		return nil, err
	}

	profile.Data = dataDir
	profile.DSN = fmt.Sprintf("%s/uamemos_%s.db", dataDir, profile.Mode)
	profile.Version = version.GetCurrentVersion(profile.Mode)

	return &profile, nil
}
