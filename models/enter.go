package models

import "errors"

type SiteGroup string

// 允许的值
const (
	CMCT  SiteGroup = "cmct"
	HDSKY SiteGroup = "hdsky"
	MTEAM SiteGroup = "mteam"
)

var allowedGroups = map[SiteGroup]struct{}{
	CMCT:  {},
	HDSKY: {},
	MTEAM: {},
}

func ValidateSiteName(value string) (SiteGroup, error) {
	group := SiteGroup(value)
	if _, ok := allowedGroups[group]; ok {
		return group, nil
	}
	return "", errors.New("invalid group value")
}
