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
	Debug(msg any, args ...any)
	Error(msg any, args ...any)
	Warn(msg any, args ...any)
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

func (s *Switcher) ListProcessNames(name string) map[string]struct{} {
	result := make(map[string]struct{}, 1)
	if name != "" {
		if s.logger != nil {
			s.logger.Debug("进程联动前台进程", "source", "foreground", "process", name)
		}
		result[strings.ToLower(strings.TrimSpace(name))] = struct{}{}
	} else if s.logger != nil {
		s.logger.Debug("进程联动前台进程为空", "source", "foreground")
	}
	return result
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
		if s.logger != nil {
			s.logger.Debug("进程联动规则检查", "process", pname, "enabled", rule.Enabled)
		}
		if _, ok := processNames[pname]; ok {
			if s.logger != nil {
				s.logger.Debug("进程联动规则命中", "process", pname, "enabled", rule.Enabled)
			}
			return &rule
		}
	}
	if s.logger != nil {
		s.logger.Debug("进程联动未命中任何规则")
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
