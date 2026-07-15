package nats

type cfg struct {
	Name            string `setting:"name" description:"The name of the nats connection"`
	Addr            string `setting:"address" description:"The address to connect to the nats server"`
	Token           string `setting:"token" description:"The token to use for authentication"`
	Secret          string `setting:"secret" description:"The token secret, when set, the client will use NKEY auth"`
	CredentialsFile string `setting:"credentials_file" description:"The path to the credentials file"`
}
