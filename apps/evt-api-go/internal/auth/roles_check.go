package auth

func HasRole(claims *AccessClaims, role Role) bool {
	for _, r := range claims.Roles {
		if r == role {
			return true
		}
	}
	return false
}
