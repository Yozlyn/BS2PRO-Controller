package procswitch

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/TIANLI0/BS2PRO-Controller/internal/types"
)

type Logger interface {
	Debug(format string, v ...any)
	Error(format string, v ...any)
	Warn(format string, v ...any)
}

type Switcher struct {
	baseDir string
	logger  Logger
}

func New(baseDir string, logger Logger) *Switcher {
	return &Switcher{baseDir: baseDir, logger: logger}
}

func (s *Switcher) ResolveProfilePath(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return ""
	}
	if filepath.IsAbs(trimmed) {
		return trimmed
	}
	return filepath.Join(s.baseDir, trimmed)
}

func (s *Switcher) ListProcessNames() (map[string]struct{}, error) {
	name, err := getForegroundProcessName()
	if err != nil {
		return nil, err
	}
	result := make(map[string]struct{}, 1)
	if name != "" {
		result[strings.ToLower(strings.TrimSpace(name))] = struct{}{}
	}
	return result, nil
}

func (s *Switcher) MatchRule(rules []types.ProcessFanRule, processNames map[string]struct{}) *types.ProcessFanRule {
	for i := range rules {
		rule := rules[i]
		if !rule.Enabled {
			continue
		}
		pname := strings.ToLower(strings.TrimSpace(rule.ProcessName))
		if pname == "" {
			continue
		}
		if _, ok := processNames[pname]; ok {
			return &rule
		}
	}
	return nil
}

func (s *Switcher) LoadCurve(path string) ([]types.FanCurvePoint, error) {
	resolved := s.ResolveProfilePath(path)
	if resolved == "" {
		return nil, fmt.Errorf("配置文件路径为空")
	}

	data, err := os.ReadFile(resolved)
	if err != nil {
		return nil, err
	}

	var wrapped struct {
		FanCurve []types.FanCurvePoint `json:"fanCurve"`
	}
	if err := json.Unmarshal(data, &wrapped); err == nil && len(wrapped.FanCurve) > 0 {
		return ensureCurve(wrapped.FanCurve), nil
	}

	var direct []types.FanCurvePoint
	if err := json.Unmarshal(data, &direct); err == nil && len(direct) > 0 {
		return ensureCurve(direct), nil
	}

	return nil, fmt.Errorf("配置文件格式无效: %s", resolved)
}

func ensureCurve(curve []types.FanCurvePoint) []types.FanCurvePoint {
	if len(curve) == 0 {
		return curve
	}
	result := make([]types.FanCurvePoint, len(curve))
	copy(result, curve)
	last := result[len(result)-1]
	if last.Temperature < 100 {
		result = append(result, types.FanCurvePoint{Temperature: 100, RPM: last.RPM, Offset: 0})
	}
	return result
}
