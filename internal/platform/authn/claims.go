package authn

// Claims is the identity from a verified authentication token.
type Claims struct {
	Sub, Email, Name string
}
