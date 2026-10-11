# Feature Specification: App-Agnostic Demo API

**Feature Branch**: `004-demo-api`

**Created**: 2026-10-11

**Status**: Draft

**Input**: User description: "Make the Metrik API app an app-agnostic Demo API. I mean loose the relationship to the Metrik app, and call it explicitely a Demo API everywhere."

## User Scenarios & Testing *(mandatory)*

The repository ships two apps: Metrik, the health checker, and a small HTTP service with live,
slow and flaky endpoints that exists only to give a health checker something to probe. Today that
service presents itself as the "Metrik API": its published contract is titled "Metrik API" and its
routes are described as the "v1 metrik api". Everything else about it (its folder, its deployable
unit, its image, its release and namespace) is called just "api".

Both names are misleading. "Metrik API" suggests it is the API *of* Metrik, a way to talk to the
health checker, which it is not. Plain "api" says nothing about it being a throwaway practice
target. This feature gives it one honest name, **Demo API**, and removes every claim that it
belongs to Metrik.

This feature has three kinds of users:
- **Readers of the Demo API's contract and documentation**, who need to understand what the
  service is from its name alone.
- **Maintainers and contributors**, who navigate the repository and need to tell the product
  (Metrik) from its practice target (the Demo API) at a glance.
- **Operators**, who deploy both apps and read their names in listings and logs.

### User Story 1 - The service presents itself as a Demo API (Priority: P1)

Someone opens the service's published contract, or reads its served contract document, and sees a
service titled "Demo API", described as a demonstration service with deliberately slow and flaky
endpoints. Nothing in it mentions Metrik.

**Why this priority**: the contract title is where the wrong relationship is stated most directly,
and it is what an outside reader sees first. Fixing it alone already removes the misleading claim.

**Independent Test**: read the contract file in the repository and the contract document served by
the running service; both are titled "Demo API" and contain no occurrence of "Metrik".

**Acceptance Scenarios**:

1. **Given** the running service, **When** a reader fetches its contract document, **Then** the
   title is "Demo API" and the description presents it as a demonstration service.
2. **Given** the contract file kept in the repository, **When** a reader searches it for "Metrik"
   in any letter case, **Then** there is no match.
3. **Given** the contract file kept in the repository and the document served by the running
   service, **When** they are compared, **Then** they describe the same title, description and
   routes.
4. **Given** the service's own source, configuration and deployment files, **When** a maintainer
   searches them for "Metrik" in any letter case, **Then** there is no match.

---

### User Story 2 - The service is named "demo" wherever it is identified (Priority: P2)

A maintainer browsing the repository, or an operator listing what is deployed, sees a name that
says "demo API" rather than a bare "api": the app's folder, its deployable package, its image, its
release and its namespace all carry the demo name.

**Why this priority**: this is the "everywhere" part of the request. It is larger and riskier than
Story 1 because other things refer to these names, so it comes second and can be delivered on its
own.

**Independent Test**: list the repository's apps and the deployed releases in the development
environment; the service appears under its demo name in each, and no bare "api" name for it
remains.

**Acceptance Scenarios**:

1. **Given** the repository, **When** a maintainer lists the apps, **Then** the service's folder
   name identifies it as the demo API.
2. **Given** a development environment deployed from the repository, **When** an operator lists
   releases, namespaces and running workloads, **Then** the service appears under its demo name.
3. **Given** the same deployment, **When** Metrik runs with the environment's shipped settings,
   **Then** its checks reach the Demo API at its new address and report the same outcomes as
   before the rename.
4. **Given** the local setup instructions, **When** a contributor follows them from a clean
   checkout, **Then** every command works as written with the new names.

---

### User Story 3 - Documentation tells the two apps apart consistently (Priority: P3)

A reader of the README or the documentation site always sees the service called "Demo API", and
always described as an independent sample target that Metrik happens to be pointed at, never as a
part of Metrik.

**Why this priority**: the documentation already mostly says "demo API". This story finishes the
job and keeps the wording uniform, but nothing breaks while it is pending.

**Independent Test**: read the README and every documentation page; each mention of the service
uses the name "Demo API", and paths, commands and addresses shown match the renamed service.

**Acceptance Scenarios**:

1. **Given** the README and documentation pages, **When** a reader looks for mentions of the
   service, **Then** each one names it "Demo API" with the same spelling and capitalisation.
2. **Given** a documentation page showing a path, command or address for the service, **When** a
   reader uses it as written, **Then** it matches the renamed service and works.
3. **Given** the architecture overview, **When** a reader looks at how the apps relate, **Then**
   the Demo API is shown as a separate target that Metrik probes, not as a Metrik component.

---

### Edge Cases

- A development or production environment still has the service deployed under its old name. The
  old release is never updated in place, and Metrik's shipped settings stop pointing at it. When
  the environment is redeployed with the project's deployment tooling, the old release is removed
  as part of that deployment. When it was installed by the development loop or by hand, it stays
  behind until removed. The upgrade note must cover both cases.
- An operator has their own Metrik settings that point at the service's old in-cluster address.
  Those checks start failing after the rename; the upgrade note must give the new address.
- The word "api" also appears where it does not name this service: the folder holding the
  contract, generic prose such as "the Kubernetes API token", and check names in example settings.
  These must not be mechanically renamed.
- Existing feature specifications (001 to 003) mention the old names. They are historical records
  and are left as written.
- The repository as a whole is still named after Metrik and still hosts the Demo API. That is
  where the service lives, not a claim made by the service, and it does not count as a remaining
  relationship.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The service MUST present itself as "Demo API" in its published contract: title,
  description and route group descriptions.
- **FR-002**: The service's own files (source, contract, settings, deployment package) MUST NOT
  mention Metrik, in any letter case.
- **FR-003**: The service MUST NOT depend on Metrik in any way: it MUST build, test, run and
  deploy on its own, without Metrik's code, settings or presence.
- **FR-004**: The contract file kept in the repository MUST match the contract the running service
  serves.
- **FR-005**: The service's behaviour MUST be unchanged: same routes, same responses, same
  deliberate slowness and flakiness, same listening port.
- **FR-006**: Every identifier that names the service MUST carry the demo name instead of a bare
  "api": the app's folder, its code module, its deployable package, its image, and its release and
  namespace in each environment.
- **FR-007**: Everything that refers to those identifiers MUST be updated in the same change so
  nothing is left pointing at an old name: local run and in-cluster development settings, the
  deployment definitions for each environment, and Metrik's shipped settings that target the
  service.
- **FR-008**: Metrik's shipped settings for each environment MUST still reach the Demo API after
  the rename, with the same checks and thresholds as before.
- **FR-009**: The README and documentation pages MUST call the service "Demo API" consistently and
  describe it as an independent sample target, and every path, command and address they show for
  it MUST match the renamed service.
- **FR-010**: The project's governing principles document MUST be updated where it names the
  service's folder or contract location, so it stays accurate.
- **FR-011**: The documentation MUST include a short upgrade note for existing environments: the
  old and new names, the new in-cluster address, and what happens to the release under the old
  name (removed on redeployment, or how to remove it by hand).
- **FR-012**: Uses of "api" that do not name this service MUST be left alone.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A case-insensitive search for "Metrik" across the service's own files returns zero
  matches.
- **SC-002**: A search of the repository for the service's old identifiers returns zero matches
  that refer to the service, outside historical feature specifications and the upgrade note,
  which names the old identifiers on purpose.
- **SC-003**: 100% of the mentions of the service in the README and documentation pages use the
  name "Demo API".
- **SC-004**: A contributor following the README from a clean checkout has the Demo API and Metrik
  running locally, with Metrik reporting check results against it, without correcting any command
  or path.
- **SC-005**: After a fresh deployment of the development environment, Metrik's shipped checks
  against the Demo API produce the same mix of outcomes as before the rename (liveness passing,
  slow endpoint breaching its threshold intermittently, readiness failing intermittently).
- **SC-006**: The service's existing automated tests pass unchanged in what they assert, and the
  project's quality checks report no new findings.
- **SC-007**: A reader shown only the service's contract can state what the service is for
  without being told about Metrik.

## Assumptions

- "Everywhere" is taken to include identifiers, not only display text: the folder, code module,
  deployable package, image, release and namespace are renamed. This is the largest scope choice in
  this specification; if only titles and descriptions were meant, Story 2 and FR-006, FR-007,
  FR-008, FR-010 and FR-011 drop out.
- The identifier form of the name is `demo-api` (for example `apps/demo-api`, release
  `dev-demo-api`); the display form is "Demo API".
- The folder holding the contract inside the app keeps its conventional name `api`, as required by
  the project layout; it names a kind of content, not the service.
- The relationship is removed in one direction only. The Demo API knows nothing about Metrik;
  Metrik's example settings and documentation still use the Demo API as their sample target.
- The Demo API stays in this repository and keeps being deployed alongside Metrik by the same
  tooling. Moving it to its own repository is out of scope.
- The service's version numbers do not change; this is a rename, not a new release of its
  behaviour.
- Existing environments are disposable local clusters, so removing the old release and deploying
  the new one is acceptable; no in-place migration or period of serving both names is needed.
- Historical feature specifications are not rewritten.
- Updating the governing principles document is a wording-only amendment and goes through the
  project's usual amendment process.
