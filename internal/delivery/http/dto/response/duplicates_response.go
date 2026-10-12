package response

// DuplicateGroupResponse is one set of same-name, same-duration/size streams.
// Keep is the newest usable member; Deletable lists the older non-published
// members the owner may remove, Protected the older published ones that are
// never deleted automatically.
type DuplicateGroupResponse struct {
	Title     string           `json:"title"`
	Count     int              `json:"count"`
	Keep      StreamResponse   `json:"keep"`
	Deletable []StreamResponse `json:"deletable"`
	Protected []StreamResponse `json:"protected"`
}

type DuplicatesResponse struct {
	Groups         []DuplicateGroupResponse `json:"groups"`
	TotalGroups    int                      `json:"total_groups"`
	TotalDeletable int                      `json:"total_deletable"`
	TotalProtected int                      `json:"total_protected"`
}
