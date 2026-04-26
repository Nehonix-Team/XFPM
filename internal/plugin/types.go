package plugin

type PendingReq struct {
	Name       string
	Identity   string
	Privileges string
}

type WebPerm struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Action      string `json:"action"`
	Description string `json:"description"`
}

type WebPlugin struct {
	Name                string    `json:"name"`
	Identity            string    `json:"identity"`
	RequestedPrivileges []WebPerm `json:"requested_privileges"`
}
