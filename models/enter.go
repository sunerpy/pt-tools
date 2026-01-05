package models

import "errors"

type SiteGroup string

// 允许的值
const (
	SpringSunday SiteGroup = "springsunday"
	HDSKY        SiteGroup = "hdsky"
	MTEAM        SiteGroup = "mteam"
	HDDOLBY      SiteGroup = "hddolby"
)

const (
	DefaultAPIUrlMTeam = "https://api.m-team.cc"
)

var allowedGroups = map[SiteGroup]struct{}{
	SpringSunday: {},
	HDSKY:        {},
	MTEAM:        {},
	HDDOLBY:      {},
}

func ValidateSiteName(value string) (SiteGroup, error) {
	group := SiteGroup(value)
	if _, ok := allowedGroups[group]; ok {
		return group, nil
	}
	return "", errors.New("invalid group value")
}
