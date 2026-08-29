package launchagent

type Status string

const (
	StatusNotRegistered    Status = "not_registered"
	StatusEnabled          Status = "enabled"
	StatusRequiresApproval Status = "requires_approval"
	StatusNotFound         Status = "not_found"
)

type Service interface {
	Register() error
	Unregister() error
	Status() Status
}
