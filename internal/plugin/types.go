package plugin

type PendingReq struct {
	Name       string
	Version    string
	Identity   string
	Privileges string
	Status     string
}

type WebPerm struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Action      string `json:"action"`
	Description string `json:"description"`
	Approved    bool   `json:"approved"`
}

type WebPlugin struct {
	Name                string    `json:"name"`
	Version             string    `json:"version"`
	Identity            string    `json:"identity"`
	Privileges          string    `json:"privileges"`
	RequestedPrivileges []WebPerm `json:"requested_privileges"`
	Status              string    `json:"status"`
}
