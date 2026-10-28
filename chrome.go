// [INPUT]: 依赖 chrome_<goos>.go 提供的 chromeDataDir（用户数据根目录）、Chrome 的 Local State 与 ui.Bitmap 头像解码。
// [OUTPUT]: 解析 Local State 得到可用 Profile 列表，如实区分文件缺失、JSON 损坏与无有效条目；不虚构占位身份。
// [POS]: 根包的 Profile 发现层；平台路径在 chrome_<goos>.go，启动在 launch*.go。
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/egoist/mygo/ui"
)

type LocalState struct {
	Profile struct {
		InfoCache map[string]struct {
			Name                string `json:"name"`
			GaiaName            string `json:"gaia_name"`
			UserName            string `json:"user_name"`
			GaiaPictureFileName string `json:"gaia_picture_file_name"`
		} `json:"info_cache"`
	} `json:"profile"`
}

// loadChromeProfiles 按当前平台的 Chrome 用户数据根目录发现 Profile。
func loadChromeProfiles() ([]Profile, error) {
	chromeBase, err := chromeDataDir()
	if err != nil {
		return nil, err
	}
	return loadProfilesFrom(chromeBase)
}

// loadProfilesFrom 读取 chromeBase 下的 Local State 并解析出可用 Profile。
//
// 关键约定：绝不返回虚构的占位 Profile。调用方必须通过 error 区分
//   - 文件不存在（Chrome 未安装 / 从未登录）
//   - JSON 解析失败（文件损坏或结构变更）
//   - 解析成功但没有任何有效 Profile（条目为空，或磁盘上目录已不存在）
//
// 这样 UI 才能如实展示空状态，而不是假装成功。
func loadProfilesFrom(chromeBase string) ([]Profile, error) {
	statePath := filepath.Join(chromeBase, "Local State")

	data, err := os.ReadFile(statePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("未找到 Chrome 配置文件：%s", statePath)
		}
		return nil, fmt.Errorf("读取 Chrome 配置失败: %w", err)
	}

	var state LocalState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("Chrome 配置文件格式无法解析: %w", err)
	}

	if len(state.Profile.InfoCache) == 0 {
		return nil, errors.New("Chrome 配置中没有任何 Profile 条目")
	}

	list := make([]Profile, 0, len(state.Profile.InfoCache))
	colorIdx := 0
	// 固定遍历顺序，保证配色分配在不同次加载之间保持稳定。
	for _, dir := range sortedDirs(state.Profile.InfoCache) {
		info := state.Profile.InfoCache[dir]

		dirPath := filepath.Join(chromeBase, dir)
		if fi, err := os.Stat(dirPath); err != nil || !fi.IsDir() {
			continue
		}

		name := strings.TrimSpace(info.Name)
		if name == "" {
			name = strings.TrimSpace(info.GaiaName)
		}
		if name == "" {
			name = strings.TrimSpace(info.UserName)
		}
		if name == "" {
			name = dir
		}

		email := strings.TrimSpace(info.UserName)

		var avatarBitmap *ui.Bitmap
		picFileName := info.GaiaPictureFileName
		if picFileName == "" {
			picFileName = "Google Profile Picture.png"
		}
		avatarPath := filepath.Join(dirPath, picFileName)
		if imgData, err := os.ReadFile(avatarPath); err == nil && len(imgData) > 0 {
			if bm, err := ui.DecodeBitmap(imgData); err == nil {
				avatarBitmap = bm
			}
		}

		list = append(list, Profile{
			Dir:    dir,
			Name:   name,
			Email:  email,
			Avatar: avatarBitmap,
			Color:  modernGradients[colorIdx%len(modernGradients)],
		})
		colorIdx++
	}

	if len(list) == 0 {
		return nil, errors.New("检测到的 Profile 条目在磁盘上均不存在")
	}

	sort.Slice(list, func(i, j int) bool {
		if list[i].Dir == "Default" {
			return true
		}
		if list[j].Dir == "Default" {
			return false
		}
		return strings.ToLower(list[i].Dir) < strings.ToLower(list[j].Dir)
	})

	return list, nil
}

// joinList 用中文顿号连接路径，避免路径里的逗号与分隔符混淆。
func joinList(items []string) string {
	return strings.Join(items, "、")
}

// sortedDirs 让 InfoCache 的 map 遍历顺序变得确定。
func sortedDirs(cache map[string]struct {
	Name                string `json:"name"`
	GaiaName            string `json:"gaia_name"`
	UserName            string `json:"user_name"`
	GaiaPictureFileName string `json:"gaia_picture_file_name"`
}) []string {
	dirs := make([]string, 0, len(cache))
	for dir := range cache {
		dirs = append(dirs, dir)
	}
	sort.Slice(dirs, func(i, j int) bool {
		if dirs[i] == "Default" {
			return true
		}
		if dirs[j] == "Default" {
			return false
		}
		return strings.ToLower(dirs[i]) < strings.ToLower(dirs[j])
	})
	return dirs
}
