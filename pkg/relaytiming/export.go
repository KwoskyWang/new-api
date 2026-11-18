package relaytiming

import "github.com/QuantumNous/new-api/common"

// ToAdminInfoValue 将快照转换为可直接放入 other.admin_info["relay_timing"] 的值。
//
// 返回的是 map[string]interface{}（经由 common JSON 包装函数往返），
// 以便与现有 other/admin_info 的序列化方式保持一致，并自动随 admin_info 一起
// 被 model.formatUserLogs 对普通用户剥离。
//
// 若快照无任何数据，返回 nil，调用方应据此跳过写入。
func (rt *RequestTiming) ToAdminInfoValue() map[string]interface{} {
	if rt == nil || !rt.HasData() {
		return nil
	}
	snap := rt.Snapshot()
	data, err := common.Marshal(snap)
	if err != nil {
		return nil
	}
	var m map[string]interface{}
	if err := common.Unmarshal(data, &m); err != nil {
		return nil
	}
	return m
}
