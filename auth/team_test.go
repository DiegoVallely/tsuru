// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"sort"

	"github.com/tsuru/tsuru/storage"
	_ "github.com/tsuru/tsuru/storage/fake"

	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

func (s *S) TestGetTeamsNames(c *check.C) {
	team := Team{Name: "cheese"}
	team2 := Team{Name: "eggs"}
	teamNames := GetTeamsNames([]Team{team, team2})
	c.Assert(teamNames, check.DeepEquals, []string{"cheese", "eggs"})
}

func (s *S) TestTeamAllowedApps(c *check.C) {
	team := Team{Name: "teamname"}
	err := storage.TeamRepository.Insert(storage.Team{Name: team.Name})
	c.Assert(err, check.IsNil)
	a := testApp{Name: "myapp", Teams: []string{s.team.Name}}
	err = s.conn.Apps().Insert(&a)
	c.Assert(err, check.IsNil)
	a2 := testApp{Name: "otherapp", Teams: []string{team.Name}}
	err = s.conn.Apps().Insert(&a2)
	c.Assert(err, check.IsNil)
	alwdApps, err := team.AllowedApps()
	c.Assert(err, check.IsNil)
	c.Assert(alwdApps, check.DeepEquals, []string{a2.Name})
}

func (s *S) TestCreateTeam(c *check.C) {
	one := User{Email: "king@pos.com"}
	err := CreateTeam("pos", &one)
	c.Assert(err, check.IsNil)
	team, err := GetTeam("pos")
	c.Assert(err, check.IsNil)
	c.Assert(team.CreatingUser, check.Equals, one.Email)
}

func (s *S) TestCreateTeamDuplicate(c *check.C) {
	u := User{Email: "king@pos.com"}
	err := CreateTeam("pos", &u)
	c.Assert(err, check.IsNil)
	err = CreateTeam("pos", &u)
	c.Assert(err, check.Equals, storage.ErrTeamAlreadyExists)
}

func (s *S) TestCreateTeamTrimsName(c *check.C) {
	u := User{Email: "king@pos.com"}
	err := CreateTeam("pos    ", &u)
	c.Assert(err, check.IsNil)
	_, err = GetTeam("pos")
	c.Assert(err, check.IsNil)
}

func (s *S) TestCreateTeamValidation(c *check.C) {
	u := User{Email: "king@pos.com"}
	var tests = []struct {
		input string
		err   error
	}{
		{"", ErrInvalidTeamName},
		{"    ", ErrInvalidTeamName},
		{"1abc", ErrInvalidTeamName},
		{"@abc", ErrInvalidTeamName},
		{"my team", ErrInvalidTeamName},
		{"Abacaxi", ErrInvalidTeamName},
		{"TEAM", ErrInvalidTeamName},
		{"TeaM", ErrInvalidTeamName},
		{"team_1", ErrInvalidTeamName},
		{"tsuru@corp.globo.com", ErrInvalidTeamName},
		{"team-1", nil},
		{"a", nil},
		{"ab", nil},
		{"team1", nil},
	}
	for _, t := range tests {
		err := CreateTeam(t.input, &u)
		if err != t.err {
			c.Errorf("Is %q valid? Want %v. Got %v.", t.input, t.err, err)
		}
	}
}

func (s *S) TestGetTeam(c *check.C) {
	team := Team{Name: "symfonia"}
	err := storage.TeamRepository.Insert(storage.Team{Name: team.Name})
	c.Assert(err, check.IsNil)
	t, err := GetTeam(team.Name)
	c.Assert(err, check.IsNil)
	c.Assert(t.Name, check.Equals, team.Name)
	t, err = GetTeam("wat")
	c.Assert(err, check.Equals, storage.ErrTeamNotFound)
	c.Assert(t, check.IsNil)
}

func (s *S) TestRemoveTeam(c *check.C) {
	team := storage.Team{Name: "atreides"}
	err := storage.TeamRepository.Insert(team)
	c.Assert(err, check.IsNil)
	err = RemoveTeam(team.Name)
	c.Assert(err, check.IsNil)
	t, err := GetTeam("atreides")
	c.Assert(err, check.Equals, storage.ErrTeamNotFound)
	c.Assert(t, check.IsNil)
}

func (s *S) TestRemoveTeamWithApps(c *check.C) {
	team := storage.Team{Name: "atreides"}
	err := storage.TeamRepository.Insert(team)
	c.Assert(err, check.IsNil)
	err = s.conn.Apps().Insert(bson.M{"name": "leto", "teams": []string{"atreides"}})
	c.Assert(err, check.IsNil)
	err = RemoveTeam(team.Name)
	c.Assert(err, check.ErrorMatches, "Apps: leto")
}

func (s *S) TestRemoveTeamWithServiceInstances(c *check.C) {
	team := storage.Team{Name: "harkonnen"}
	err := storage.TeamRepository.Insert(team)
	c.Assert(err, check.IsNil)
	err = s.conn.ServiceInstances().Insert(bson.M{"name": "vladimir", "teams": []string{"harkonnen"}})
	c.Assert(err, check.IsNil)
	err = RemoveTeam(team.Name)
	c.Assert(err, check.ErrorMatches, "Service instances: vladimir")
}

func (s *S) TestListTeams(c *check.C) {
	err := storage.TeamRepository.Insert(storage.Team{Name: "corrino"})
	c.Assert(err, check.IsNil)
	err = storage.TeamRepository.Insert(storage.Team{Name: "fenring"})
	c.Assert(err, check.IsNil)
	teams, err := ListTeams()
	c.Assert(err, check.IsNil)
	c.Assert(teams, check.HasLen, 3)
	names := []string{teams[0].Name, teams[1].Name, teams[2].Name}
	sort.Strings(names)
	c.Assert(names, check.DeepEquals, []string{"cobrateam", "corrino", "fenring"})
}
