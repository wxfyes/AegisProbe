package routequality

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"tianque_probe/internal/common"
	"tianque_probe/internal/server/config"
	"tianque_probe/internal/server/store"
)

type Checker struct {
	cfgMgr      *config.Manager
	store       *store.Store
	client      *http.Client
	mu          sync.Mutex
	running     bool
	alertMu     sync.Mutex
	alertStates map[string]routeAlertState
	alertFn     func(string)
}

type routeAlertState struct {
	badRounds int
	alerted   bool
}

func NewChecker(cfgMgr *config.Manager, st *store.Store) *Checker {
	return &Checker{cfgMgr: cfgMgr, store: st, client: &http.Client{Timeout: 15 * time.Second}, alertStates: make(map[string]routeAlertState)}
}

func (c *Checker) SetAlertCallback(fn func(string)) { c.alertFn = fn }

func (c *Checker) Start(ctx context.Context) {
	go func() {
		var next time.Time
		var lastInterval int
		tick := time.NewTicker(5 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				cfg := c.cfgMgr.GetConfig().RouteQuality
				if !cfg.Enabled {
					next = time.Time{}
					lastInterval = 0
					continue
				}
				interval := cfg.IntervalSec
				if interval < 60 {
					interval = 1800
				}
				if lastInterval != 0 && interval != lastInterval {
					// A settings save already triggers the updated round. Re-anchor
					// the next periodic run to the newly selected interval.
					next = time.Now().Add(time.Duration(interval) * time.Second)
				}
				lastInterval = interval
				if next.IsZero() || !time.Now().Before(next) {
					c.Trigger()
					next = time.Now().Add(time.Duration(interval) * time.Second)
				}
			}
		}
	}()
}

func (c *Checker) Trigger() bool {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return false
	}
	c.running = true
	c.mu.Unlock()
	go func() {
		defer func() { c.mu.Lock(); c.running = false; c.mu.Unlock() }()
		c.run()
	}()
	return true
}

func (c *Checker) IsRunning() bool { c.mu.Lock(); defer c.mu.Unlock(); return c.running }

func (c *Checker) run() {
	cfg := c.cfgMgr.GetConfig()
	settings := cfg.RouteQuality
	if !settings.Enabled {
		return
	}
	nodes := cfg.Nodes
	type job struct {
		nodeID   string
		nodeName string
		target   string
		carrier  carrierConfig
	}
	jobs := make([]job, 0, len(nodes)*3)
	for _, node := range nodes {
		ipv4, ipv6, _, _, online := c.store.GetNodeRealInfo(node.ID)
		if !online {
			continue
		}
		target := ipv4
		if target == "" {
			target = ipv6
		}
		if net.ParseIP(target) == nil {
			continue
		}
		for _, carrier := range carriers(settings) {
			jobs = append(jobs, job{nodeID: node.ID, nodeName: node.Name, target: target, carrier: carrier})
		}
	}
	if len(jobs) == 0 {
		return
	}
	if settings.Provider != "custom" {
		planned := len(jobs) * settings.ProbeCount
		if remaining, ok := c.globalpingRemaining(settings); ok && remaining < planned {
			message := fmt.Sprintf("额度不足：剩余 %d 次，本轮预计需要 %d 次", remaining, planned)
			checkedAt := time.Now().Format(time.RFC3339)
			for _, j := range jobs {
				c.store.UpdateRouteQuality(j.nodeID, common.CarrierQuality{Carrier: j.carrier.name, Status: "error", Message: message, CheckedAt: checkedAt})
			}
			log.Printf("[三网质量] 已跳过本轮，%s", message)
			return
		}
	}
	sem := make(chan struct{}, 3)
	var wg sync.WaitGroup
	for _, j := range jobs {
		sem <- struct{}{}
		wg.Add(1)
		go func(j job) {
			defer wg.Done()
			defer func() { <-sem }()
			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(settings.TimeoutSec+15)*time.Second)
			defer cancel()
			var result common.CarrierQuality
			if settings.Provider == "custom" {
				result = c.custom(ctx, settings, j.target, j.carrier)
			} else {
				result = c.globalping(ctx, settings, j.target, j.carrier)
			}
			result.Carrier = j.carrier.name
			result.CheckedAt = time.Now().Format(time.RFC3339)
			c.store.UpdateRouteQuality(j.nodeID, result)
			c.evaluateAlert(j.nodeID, j.nodeName, result, settings)
		}(j)
	}
	wg.Wait()
}

func (c *Checker) evaluateAlert(nodeID, nodeName string, quality common.CarrierQuality, cfg common.RouteQualityConfig) {
	key := nodeID + ":" + quality.Carrier
	c.alertMu.Lock()
	state := c.alertStates[key]
	if !cfg.AlertEnabled {
		delete(c.alertStates, key)
		c.alertMu.Unlock()
		return
	}
	if quality.Status != "ok" && quality.Status != "loss" {
		state.badRounds = 0
		c.alertStates[key] = state
		c.alertMu.Unlock()
		return
	}
	bad := quality.LatencyMS >= cfg.AlertLatencyMS || quality.LossPct >= cfg.AlertLossPct
	var message string
	if bad {
		state.badRounds++
		if state.badRounds >= 2 && !state.alerted {
			state.alerted = true
			message = fmt.Sprintf("🔴 <b>【三网线路质量告警】</b>\n<b>节点</b>：%s\n<b>线路</b>：%s\n<b>延迟</b>：%.1f ms（阈值 %.1f ms）\n<b>丢包</b>：%.1f%%（阈值 %.1f%%）\n已连续两轮达到异常阈值。", html.EscapeString(nodeName), html.EscapeString(quality.Carrier), quality.LatencyMS, cfg.AlertLatencyMS, quality.LossPct, cfg.AlertLossPct)
		}
	} else {
		state.badRounds = 0
		if state.alerted {
			state.alerted = false
			message = fmt.Sprintf("🟢 <b>【三网线路质量恢复】</b>\n<b>节点</b>：%s\n<b>线路</b>：%s\n<b>延迟</b>：%.1f ms\n<b>丢包</b>：%.1f%%", html.EscapeString(nodeName), html.EscapeString(quality.Carrier), quality.LatencyMS, quality.LossPct)
		}
	}
	c.alertStates[key] = state
	callback := c.alertFn
	c.alertMu.Unlock()
	if message != "" && callback != nil {
		callback(message)
	}
}

func (c *Checker) globalpingRemaining(cfg common.RouteQualityConfig) (int, bool) {
	base := strings.TrimRight(cfg.APIBaseURL, "/")
	if base == "" {
		base = "https://api.globalping.io"
	}
	req, err := http.NewRequest(http.MethodGet, base+"/v1/limits", nil)
	if err != nil {
		return 0, false
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "TianQue-Probe/1.0 (ICMP route quality)")
	if cfg.APIToken != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.APIToken)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		log.Printf("[三网质量] 额度预检失败，继续执行检测: %v", err)
		return 0, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Printf("[三网质量] 额度预检返回 HTTP %d，继续执行检测", resp.StatusCode)
		return 0, false
	}
	var limits struct {
		RateLimit struct {
			Measurements struct {
				Create struct {
					Remaining int `json:"remaining"`
				} `json:"create"`
			} `json:"measurements"`
		} `json:"rateLimit"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 64*1024)).Decode(&limits) != nil {
		return 0, false
	}
	return limits.RateLimit.Measurements.Create.Remaining, true
}

type carrierConfig struct{ key, name, magic string }

func carriers(c common.RouteQualityConfig) []carrierConfig {
	return []carrierConfig{{"telecom", "电信", c.TelecomMagic}, {"unicom", "联通", c.UnicomMagic}, {"mobile", "移动", c.MobileMagic}}
}

type measurementRequest struct {
	Type      string `json:"type"`
	Target    string `json:"target"`
	Locations []struct {
		Magic string `json:"magic"`
		Limit int    `json:"limit"`
	} `json:"locations"`
	MeasurementOptions struct {
		Protocol string `json:"protocol"`
		Packets  int    `json:"packets"`
	} `json:"measurementOptions"`
}
type measurementResponse struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Results []struct {
		Probe struct {
			ASN     int    `json:"asn"`
			Network string `json:"network"`
		} `json:"probe"`
		Result struct {
			Status string `json:"status"`
			Stats  struct {
				Avg  *float64 `json:"avg"`
				Loss *float64 `json:"loss"`
			} `json:"stats"`
		} `json:"result"`
	} `json:"results"`
}

func (c *Checker) globalping(ctx context.Context, cfg common.RouteQualityConfig, target string, carrier carrierConfig) common.CarrierQuality {
	base := strings.TrimRight(cfg.APIBaseURL, "/")
	if base == "" {
		base = "https://api.globalping.io"
	}
	reqBody := measurementRequest{Type: "ping", Target: target}
	reqBody.Locations = append(reqBody.Locations, struct {
		Magic string `json:"magic"`
		Limit int    `json:"limit"`
	}{carrier.magic, cfg.ProbeCount})
	reqBody.MeasurementOptions.Protocol = "ICMP"
	reqBody.MeasurementOptions.Packets = cfg.Packets
	body, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/v1/measurements", bytes.NewReader(body))
	if err != nil {
		return qualityError("请求构建失败")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "TianQue-Probe/1.0 (ICMP route quality)")
	if cfg.APIToken != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.APIToken)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		log.Printf("[三网质量] Globalping 创建测量失败 (%s): %v", carrier.name, err)
		return qualityError("接口连接失败")
	}
	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		resp.Body.Close()
		detail := globalpingErrorDetail(msg)
		log.Printf("[三网质量] Globalping 创建测量失败 (%s): HTTP %d %s", carrier.name, resp.StatusCode, detail)
		if resp.StatusCode == http.StatusTooManyRequests {
			return common.CarrierQuality{Status: "error", Message: "Globalping 每小时额度或频率限制 (HTTP 429)"}
		}
		if resp.StatusCode == http.StatusBadRequest {
			return qualityError("Globalping 请求参数错误: " + detail)
		}
		return qualityError(fmt.Sprintf("Globalping 接口返回 HTTP %d: %s", resp.StatusCode, detail))
	}
	var created measurementResponse
	location := resp.Header.Get("Location")
	responseBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	resp.Body.Close()
	if readErr != nil || json.Unmarshal(responseBody, &created) != nil {
		return qualityError("接口未返回测量编号")
	}
	if created.ID == "" && location != "" {
		parsed, err := url.Parse(location)
		if err == nil {
			created.ID = strings.Trim(strings.TrimRight(parsed.Path, "/"), "/")
			if slash := strings.LastIndexByte(created.ID, '/'); slash >= 0 {
				created.ID = created.ID[slash+1:]
			}
		}
	}
	if created.ID == "" {
		return qualityError("接口未返回测量编号")
	}
	measurementURL := base + "/v1/measurements/" + url.PathEscape(created.ID)
	deadline := time.Now().Add(time.Duration(cfg.TimeoutSec) * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return qualityError("检测超时")
		case <-time.After(1200 * time.Millisecond):
		}
		poll, err := http.NewRequestWithContext(ctx, http.MethodGet, measurementURL, nil)
		if err != nil {
			return qualityError("结果请求构建失败")
		}
		poll.Header.Set("Accept", "application/json")
		poll.Header.Set("User-Agent", "TianQue-Probe/1.0 (ICMP route quality)")
		if cfg.APIToken != "" {
			poll.Header.Set("Authorization", "Bearer "+cfg.APIToken)
		}
		got, err := c.client.Do(poll)
		if err != nil {
			continue
		}
		if got.StatusCode != http.StatusOK {
			got.Body.Close()
			continue
		}
		var measurement measurementResponse
		err = json.NewDecoder(io.LimitReader(got.Body, 512*1024)).Decode(&measurement)
		got.Body.Close()
		if err != nil {
			continue
		}
		if measurement.Status == "in-progress" {
			continue
		}
		return aggregate(measurement.Results, carrier.name)
	}
	return qualityError("等待 Globalping 结果超时")
}

func globalpingErrorDetail(body []byte) string {
	var payload struct {
		Error struct {
			Message string            `json:"message"`
			Params  map[string]string `json:"params"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &payload) == nil {
		if len(payload.Error.Params) > 0 {
			parts := make([]string, 0, len(payload.Error.Params))
			for key, value := range payload.Error.Params {
				parts = append(parts, key+": "+value)
			}
			sort.Strings(parts)
			return strings.Join(parts, "; ")
		}
		if payload.Error.Message != "" {
			return payload.Error.Message
		}
	}
	detail := strings.TrimSpace(string(body))
	if detail == "" {
		return "服务方未提供错误详情"
	}
	return detail
}

func aggregate(results []struct {
	Probe struct {
		ASN     int    `json:"asn"`
		Network string `json:"network"`
	} `json:"probe"`
	Result struct {
		Status string `json:"status"`
		Stats  struct {
			Avg  *float64 `json:"avg"`
			Loss *float64 `json:"loss"`
		} `json:"stats"`
	} `json:"result"`
}, carrier string) common.CarrierQuality {
	var latencies []float64
	var lossSum float64
	var count int
	network := ""
	for _, item := range results {
		if item.Result.Status != "finished" {
			continue
		}
		if item.Result.Stats.Loss == nil {
			continue
		}
		lossSum += *item.Result.Stats.Loss
		if item.Result.Stats.Avg != nil && *item.Result.Stats.Avg > 0 {
			latencies = append(latencies, *item.Result.Stats.Avg)
		}
		if item.Probe.Network != "" {
			network = item.Probe.Network
		}
		count++
	}
	if count == 0 {
		return common.CarrierQuality{Status: "unavailable", Message: "暂时没有符合条件的在线探针"}
	}
	loss := lossSum / float64(count)
	quality := common.CarrierQuality{Status: "ok", LossPct: loss, ProbeCount: count, Network: network}
	if len(latencies) > 0 {
		for _, value := range latencies {
			quality.LatencyMS += value
		}
		quality.LatencyMS /= float64(len(latencies))
	} else {
		quality.Status = "loss"
	}
	if loss >= 100 {
		quality.Status = "loss"
	}
	return quality
}

func (c *Checker) custom(ctx context.Context, cfg common.RouteQualityConfig, target string, carrier carrierConfig) common.CarrierQuality {
	endpoint := strings.NewReplacer("{ip}", url.QueryEscape(target), "{carrier}", url.QueryEscape(carrier.key), "{filter}", url.QueryEscape(carrier.magic), "{probe_count}", strconv.Itoa(cfg.ProbeCount), "{timeout}", strconv.Itoa(cfg.TimeoutSec), "{packets}", strconv.Itoa(cfg.Packets)).Replace(cfg.CustomURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return qualityError("自定义接口地址无效")
	}
	req.Header.Set("Accept", "application/json")
	if cfg.APIToken != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.APIToken)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return qualityError("自定义接口连接失败")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return qualityError(fmt.Sprintf("自定义接口返回 HTTP %d", resp.StatusCode))
	}
	var payload struct {
		LatencyMS *float64 `json:"latency_ms"`
		LossPct   *float64 `json:"loss_percent"`
		Status    string   `json:"status"`
		Network   string   `json:"network"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64*1024)).Decode(&payload); err != nil {
		return qualityError("返回内容不是约定的 JSON 格式")
	}
	if payload.LatencyMS == nil || payload.LossPct == nil {
		return qualityError("JSON 必须包含 latency_ms 和 loss_percent 数值")
	}
	status := "ok"
	if *payload.LossPct >= 100 {
		status = "loss"
	}
	return common.CarrierQuality{Status: status, LatencyMS: *payload.LatencyMS, LossPct: *payload.LossPct, ProbeCount: cfg.ProbeCount, Network: payload.Network}
}

func qualityError(message string) common.CarrierQuality {
	return common.CarrierQuality{Status: "error", Message: message}
}
