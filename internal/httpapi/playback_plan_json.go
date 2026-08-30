package httpapi

import "encoding/json"

// UnmarshalJSON intentionally keeps PlaybackPlan requests forward-compatible.
// Mobile/TV clients report device capabilities and those payloads can gain
// additive fields independently from the server version. The generic API
// decoder uses DisallowUnknownFields, which is useful for mutation endpoints,
// but it made a valid Android capability handshake fail with the misleading
// "invalid JSON body" error whenever a newer client/device reported an extra
// capability. Delegating this one request type to json.Unmarshal preserves
// strict JSON syntax while ignoring unknown additive fields, like mature media
// server protocols do for client capability negotiation.
func (p *playbackPlanRequest) UnmarshalJSON(data []byte) error {
	type alias playbackPlanRequest
	return json.Unmarshal(data, (*alias)(p))
}
