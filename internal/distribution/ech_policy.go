package distribution

import (
 "context"
 "crypto/sha256"
 "encoding/base64"
 "encoding/binary"
 "encoding/hex"
 "encoding/json"
 "fmt"
 "maps"
 "net/netip"
 "net/url"
 "strings"
 "time"

 "github.com/Eiyuuzaki/paily-on-nas/third_party/paily-fetch/node"
 "github.com/google/uuid"
)

const echEvidenceMaxAge = 3 * time.Hour

// echAssessment is a direct checker observation, never a field read from source raw.
// Keep this wire shape aligned with paily-check/client.ECHAssessment.
type echAssessment struct {
 Version int `json:"version"`
 Decision string `json:"decision"`
 RawSHA256 string `json:"raw_sha256"`
 ProbeURL string `json:"probe_url"`
 PlainAttempts int `json:"plain_attempts"`
 PlainSuccess int `json:"plain_success"`
 ECHAttempts int `json:"ech_attempts"`
 ECHSuccess int `json:"ech_success"`
 Config string `json:"config"`
 CheckedAt time.Time `json:"checked_at"`
}

type echProofRow struct {
 NodeID uuid.UUID
 Metadata string
 CheckedAt time.Time
}

func echRawSHA256(raw map[string]any) (string, error) {
 clone := maps.Clone(raw)
 delete(clone,"name")
 data, err := json.Marshal(clone)
 if err != nil { return "", err }
 sum := sha256.Sum256(data)
 return hex.EncodeToString(sum[:]), nil
}

func validECHConfig(config string) bool {
 data, err := base64.StdEncoding.DecodeString(config)
 return err == nil && len(data) >= 6 && len(data) <= 65537 && int(binary.BigEndian.Uint16(data[:2])) == len(data)-2
}

func hasSourceECH(raw map[string]any) bool {
 _, opts := raw["ech-opts"]
 _, pinned := raw["ech"]
 return opts || pinned
}

func parseECHProof(row echProofRow, now time.Time) (echAssessment, bool) {
 var metadata struct { ECH *echAssessment `json:"ech_assessment"` }
 if json.Unmarshal([]byte(row.Metadata), &metadata) != nil || metadata.ECH == nil { return echAssessment{}, false }
 p := *metadata.ECH
 if p.Version != 1 || p.Decision != "required" || p.ProbeURL != "https://www.google.com/generate_204" { return p,false }
 if p.PlainAttempts != 2 || p.PlainSuccess != 0 || p.ECHAttempts != 2 || p.ECHSuccess != 2 { return p,false }
 for _, at := range []time.Time{row.CheckedAt,p.CheckedAt} {
  if at.IsZero() || at.After(now.Add(time.Minute)) || now.Sub(at) > echEvidenceMaxAge { return p,false }
 }
 return p, !p.CheckedAt.After(row.CheckedAt.Add(time.Minute)) && validECHConfig(p.Config)
}

// latest initial -> same run's direct deep prevents an older ECH success from
// overriding a newer plaintext success, failed check, or a legacy checker report.
func (h *Handler) loadECHProofs(ctx context.Context, selected []SelectedNode, tag string) (map[uuid.UUID]echAssessment, error) {
 proofs := make(map[uuid.UUID]echAssessment)
 if configInt(h.db,"ech_enabled",1) == 0 { return proofs,nil }
 ids := make([]uuid.UUID,0,len(selected))
 for _, sn := range selected {
  if sn.Entry != nil && sn.Entry.ProxyNode != nil && echEligible(sn.Entry.ProxyNode.ClashMap()) { ids=append(ids,sn.Entry.ID) }
 }
 if len(ids)==0 { return proofs,nil }
 if tag=="" { tag=configString(h.db,"ech_checker_tag","local") }
 now := time.Now().UTC()
 cutoff := now.Add(-echEvidenceMaxAge)
 var rows []echProofRow
 err := h.db.WithContext(ctx).Raw(`
WITH latest AS (
 SELECT node_id, check_run_id, latency_ms, checked_at,
        ROW_NUMBER() OVER (PARTITION BY node_id ORDER BY checked_at DESC, id DESC) AS seq
 FROM node_initial_checks
 WHERE node_id IN ? AND checker_tag = ? AND checked_at >= ?
), paired AS (
 SELECT i.node_id, d.metadata, i.checked_at, d.avg_latency_ms,
        ROW_NUMBER() OVER (PARTITION BY i.node_id ORDER BY d.checked_at DESC,d.id DESC) AS seq
 FROM latest i JOIN node_deep_checks d
  ON d.node_id=i.node_id AND d.check_run_id=i.check_run_id AND d.checker_tag=?
 WHERE i.seq=1 AND i.latency_ms >= 0 AND d.source='direct' AND d.checked_at >= ?
)
SELECT node_id, metadata, checked_at FROM paired WHERE seq=1 AND avg_latency_ms >= 0`, ids, tag, cutoff, tag, cutoff).Scan(&rows).Error
 if err != nil { return nil,fmt.Errorf("load ECH necessity evidence: %w",err) }
 for _, row:=range rows { if p,ok:=parseECHProof(row,now);ok { proofs[row.NodeID]=p } }
 return proofs,nil
}

// Keep the supported transport scope aligned with the checker A/B probe.
func echEligible(raw map[string]any) bool {
 if hasSourceECH(raw) || raw["tls"] != true || raw["network"] != "ws" || raw["security"] == "reality" { return false }
 if typ, _ := raw["type"].(string); typ != "vless" && typ != "vmess" { return false }
 for _, key := range []string{"reality-opts", "shadow-tls-opts", "restls-opts", "jls-opts", "dialer-proxy", "smux"} {
  if _, found := raw[key]; found { return false }
 }
 if ws, ok := raw["ws-opts"].(map[string]any); ok {
  for _, key := range []string{"max-early-data", "v2ray-http-upgrade-fast-open"} {
   if _, found := ws[key]; found { return false }
  }
  path, _ := ws["path"].(string)
  if parsed, err := url.Parse(path); err != nil || parsed.Query().Has("ed") { return false }
 }
 server, _ := raw["server"].(string)
 if _, err := netip.ParseAddr(server); err != nil { return false }
 host, _ := raw["servername"].(string)
 host = strings.TrimSuffix(host,".")
 _, err := netip.ParseAddr(host)
 return err != nil && strings.Contains(host, ".") && !strings.ContainsAny(host, " /:?&#\t\n")
}

func proxyWithRequiredECH(orig *node.ProxyNode, proof echAssessment) (*node.ProxyNode,error) {
 if orig==nil { return nil,nil }
 raw:=orig.ClashMap()
 if !echEligible(raw) || proof.Decision!="required" { return orig,nil }
 hash,err:=echRawSHA256(raw)
 if err!=nil { return nil,fmt.Errorf("fingerprint ECH node: %w",err) }
 if proof.Decision!="required" || proof.RawSHA256!=hash || !validECHConfig(proof.Config) { return orig,nil }
 clone:=maps.Clone(raw)
 clone["ech-opts"]=map[string]any{"enable":true,"config":proof.Config}
 out,err:=node.New(clone)
 if err!=nil { return nil,fmt.Errorf("construct ECH node: %w",err) }
 return out,nil
}
