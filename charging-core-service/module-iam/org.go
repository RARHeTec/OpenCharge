package iam

// OrgNode is one level of the platform's tree:
// Platform Root -> Partner -> Operator(Tenant) -> Station Admin -> Role.
type OrgNode struct {
	ID       string
	ParentID string
	Type     string // root | partner | operator | station | role
	TenantID string
}

// IsDescendant reports whether child is under parent in the tree. Used by ABAC
// to constrain a station admin to their own stations.
func IsDescendant(nodes []OrgNode, parentID, childID string) bool {
	parents := map[string]string{}
	for _, n := range nodes {
		parents[n.ID] = n.ParentID
	}
	cur := childID
	seen := map[string]bool{}
	for cur != "" && !seen[cur] {
		if cur == parentID {
			return true
		}
		seen[cur] = true
		cur = parents[cur]
	}
	return false
}
