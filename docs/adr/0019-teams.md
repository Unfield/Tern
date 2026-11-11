# 0019. Teams and the audit log in accounts

- Status: Accepted
- Date: 2026-11-11

## Context

Most SaaS apps sell to groups, not to people: a company signs up, invites colleagues, and gives some of them more rights than others. The saas template kept each project to one user, and every app had to build the rest: who is in which team, invitations by email that only the invited person can accept, a role per member, and the rule that a team never loses its last owner. This is easy to get subtly wrong, and wrong here means someone sees another company's data.

Support and security questions ("who removed Bob?", "was I signed in from somewhere else?") need a record of what happened to accounts, which apps also had to build.

## Decision

- **Teams live in `accounts`,** not in a package of their own. Membership is part of who the user is in a request, and it shares the users table, the email sending and the sessions. Each session remembers its current team; `accounts.CurrentTeam(ctx)` loads it on first use, so requests that don't ask pay nothing.
- **Three built-in roles,** owner, admin and member, with fixed rules: owners and admins manage members and invitations, only owners make or remove owners and delete the team, and the last owner can't leave or be demoted. Apps add roles of their own (`Options.TeamRoles`) and check permissions with `rbac` through `accounts.TeamRoles`. The built-in rules don't depend on an app's rbac policy, so a policy mistake can't lock a team out or let a member take it over.
- **Invitations are emailed links,** valid for a week, and only the invited address accepts them: the signed-in user's email must match. A link to the wrong inbox, forwarded or leaked, gives nothing. Accepting verifies the address.
- **The app's data is the app's.** `Options.OnDeleteTeam` runs before a team is deleted, to delete its rows in the app's tables. Deleting the last member's account deletes the team; deleting the last owner of a team with other members is refused.
- **An audit log in the same database,** written by accounts for its own events and by apps through `acc.Audit`. Users see their own entries, team admins their team's, and support tools anyone's through `acc.AuditEntries`. Entries are kept for a year by default. Recording never fails the operation it records: errors are logged.

## Consequences

- A SaaS app gets teams, invitations and roles with a few calls, the same on SQLite and Postgres.
- The rules of the built-in roles are fixed. An app that needs, say, admins who can't invite builds that check in its own action before calling `Invite`.
- A user works in one team at a time. Data from several teams at once (a dashboard across teams) is the app's to query.
- The audit log grows with sign-ins. A year of a busy app's sign-ins is still small next to its own data; `AuditFor` shortens it, and a negative value turns it off.
