package nats

type cfg struct {
	NATS *natsConfig `config:"nats,block"`
}

type natsConfig struct {
	Name            string `config:"name,optional" env:"NATS_NAME" description:"The name of the nats connection"`
	Addr            string `config:"address,label" env:"NATS_ADDRESS" setting:"Address" description:"The address to connect to the nats server"`
	Token           string `config:"token,optional" env:"NATS_TOKEN" setting:"Token" description:"The token to use for authentication"`
	Secret          string `config:"secret,optional" env:"NATS_SECRET" setting:"Secret" description:"The token secret, when set, the client will use NKEY auth"`
	CredentialsFile string `config:"credentials_file,optional" env:"NATS_CREDENTIALS_FILE" setting:"CredentialsFile" description:"The path to the credentials file"`
}
