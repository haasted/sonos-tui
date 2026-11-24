# appModel refactor: analysis and proposal

Status: proposal for review. No code is changed. Line counts and field counts are from `main` at 694d891.

## 1. Summary

`appModel` has 46 fields and 113 methods in 11 files. Its `update` switch has 60 cases, and 53 of them only call a handler. Each feature keeps its state as loose fields on `appModel`. Features call each other through `appModel` methods. Nothing stops one feature from changing the state of another.

The code already splits most features into two halves:

- **content** (`searchContent`, `queueContent`, `signInContent` …). This is a pure `tea.Model` inside a `panes.Modal`. It sends *request messages* such as `searchAskMsg` and `queueRemoveMsg`. This half is good. Do not change it.
- **control** (`searchcontrol.go`, `queuecontrol.go` …). These are methods on `appModel`. They do the I/O and handle the results. This half is the problem.

**Main recommendation.** Make each control half a *feature component*: a value type in `package main` that owns its modal, its generation counters and its own state, and has one `update` method. `appModel` keeps the speakers, the frame, the overlay order and the services. It also becomes the only place that connects two features.

A feature gets what it cannot know through a small read-only `appEnv` value that `appModel` builds for each update. It asks the app to do things through a returned `appAsk` value. It does not use messages for this. Each feature claims its messages with a marker interface. Then `update` has one case for each feature, not one case for each message.

Do this in about 12 small commits. Start with the list modal type (son-akp.4), then search, sign-in and settings, then the other lists, then the overlay order. Each commit keeps `make vet test` green and changes no behaviour. At the end, `appModel` has about 16 fields.

Do not move features into their own packages yet. Move only the generic list code (son-akp.2). Section 6 tells why.

## 2. Current structure

### 2.1 Features against state and methods

| Feature | Fields on appModel | appModel methods | Files |
|---|---|---|---|
| Speakers and tabs | `ctx`, `tabs`, `ids`, `eventManager` | 16: `insertSpeaker`, `activeSpeaker`, `deviceFor`, `applyEvent`, `applySpeakerMsg`, `syncPlaying`, `leaderFor` … | appModel.go |
| Presence and discovery | `discoveryDone`, `discovery`, `fromCache`, `cached`, `cachedStates`, `found` | 4 + `Init` | presence.go |
| Topology and grouping | `household`, `askedTopology`, `askingTopology`, `grouping` | 3: `readUncovered`, `openGrouping`, `applyGroupChange` | appModel.go, grouping.go |
| Frame and terminal | `width`, `height`, `sized`, `frameRows`, `screenTop`, `quitting` | 7: `growFrame`, `settleScreenTop`, `frameHeight`, `backdrop`, `fits`, `tooSmallView`, `View` | appModel.go, fit.go |
| Help, hint, border light | `help`, `hintDone`, `sweepGen` | 5: `openHelp`, `applyHintTick`, `applyModalSweepDue/Tick`, `restartSweepChain` | hint.go, queuecontrol.go |
| List modal plumbing | `queueGen`, `queueHintGen`, `favouritesGen`, `favouritesHintGen`, `searchGen`, `searchHintGen` | 17 | listmodal.go |
| Queue | `queue` | 11 | queuecontrol.go |
| Favourites and mark chooser | `favourites`, `marking`, `db` | 13 | favouritescontrol.go |
| Search, browsing, guesses | `search`, `searchRun`, `searchCancel`, `classifier`, `findClassifier` | 16 | searchcontrol.go, searchguess.go |
| Cloud link | `cloud`, `cloudAsked`, `localHousehold` | 2: `lookUpHousehold`, `applyCloudHousehold` | searchcontrol.go |
| Sign-in | `signIn`, `signInGen`, `signInThen` | 10 | signincontrol.go |
| Settings | `settings`, `settingsGen` | 11 | settingscontrol.go |

13 of the 46 fields are generation counters.

### 2.2 Cross-feature dependencies

| From | Needs or changes | Where |
|---|---|---|
| Search | the active speaker's device (household fallback) and `speakerModel` (to play) | `applySearchAsk`, `applySearchAction` |
| Search | `cloud`, `localHousehold`, `classifier` | searchcontrol.go |
| Search | opens the sign-in (`followSearch`) on a signed-out error | 5 places |
| Search | `db` and the favourites helper `storeFavourites` | `applySearchMark` |
| Sign-in | on success, calls `openSearch` and `applySearchAsk`, or `settingsSignedIn` | `afterSignIn` |
| Settings | opens the sign-in (`followSettings`); on sign-out it clears `search`, `searchRun`, `searchCancel` | settingscontrol.go |
| Speakers | starts the cloud household lookup and the topology read when a speaker loads | `speakerReadyMsg`, `applyVerified` |
| Events | move the open queue's marker (`markPlaying`) and read queues again | `applyEvent` |
| Queue, player `a` | send `favouriteChooseMsg`, which can open the mark chooser | favouritescontrol.go |
| Notices | go to the open list's border, else to the tab row; they also retire the hint | `NoticeMsg` case |
| Quit key | must know `listTyping()` and whether the sign-in is open | `KeyMsg` case |
| Mouse | `listTakesTheWheel` reads help, sign-in, settings; a click reads `backdrop()`, `screenTop`, marking | listmodal.go |
| Frame height, size, view | read all 8 modals, each listed by name | `growFrame`, `WindowSizeMsg`, `View` |

### 2.3 Patterns already in use (keep them)

- **Content and control split.** The content sends request messages, and the control does the I/O. This is the Elm way, and it makes content easy to test.
- **Generation counters.** `nextGen()` is one process-wide atomic counter. `listTickMsg{gen}` already uses the fact that a generation alone names its chain. Bubbles does the same with `spinner.TickMsg{ID}`.
- **Border notices and hints.** `Modal.Notify`, `NoticeShowing`, `NoticeInterval` and a tick chain.
- **`listContent` interface.** The app drives three lists without knowing which list it is.
- **`panes.EscClaimer`.** The modal asks the content, so `panes` does not know the content.
- **Central post-update** (`syncPlaying`, `growFrame`, `settleScreenTop`). One path cannot forget it.
- **Value semantics.** All models are values. Tests copy models freely.

## 3. Problems, ranked

1. **No boundaries between features.** One feature calls another feature's methods and writes its fields. Examples: `afterSignIn` runs `openSearch` and `applySearchAsk`; `applySettingsSignedOut` resets three search fields. Each new flow adds a new path through `appModel`.
2. **Feature state is loose fields with a naming rule.** `xGen`, `xHintGen`, `xRun`, `xCancel`. A new list modal or bordered modal adds 2–3 fields and edits 4–6 places.
3. **The update switch.** 60 cases, 53 only forward. Every feature edits appModel.go.
4. **Copies of the same code.**
   - The sign-in and settings border chains are the same code with different names: 4 message types, 6 methods.
   - `*slot.pulse, *slot.hint = nextGen(), nextGen()` is in 3 places.
   - `if msg.localID != "" { m.localHousehold = … }` is in 4 places, and 4 message types carry `localID`.
   - The guard "cloud nil, or signed out, then open the sign-in" is in 3 places.
5. **The modals are listed by name in 6 places**, and the orders are different: View (draw), the key chain, `growFrame`, `WindowSizeMsg`, `listTakesTheWheel`, `listTyping`.
6. **`listSlot` holds pointers into a value receiver.** It is correct, but easy to misuse (son-akp.4).
7. **A possible inconsistency.** View draws help *over* the sign-in, but the sign-in gets the keys *first*. If a search fails with "signed out" while help is open over the search, then the sign-in opens under the help and takes the keys. I did not test this. See open question 3.

These parts are fine as they are: the content types, `panes.Modal`, the notices through `panes.NoticeMsg`, `speakerMsg` (a message that carries its target), and the central `syncPlaying`.

## 4. Proposed structure

### 4.1 What appModel keeps

```go
type appModel struct {
	ctx      context.Context
	services services   // db, cloud link, event manager, classifier finder
	tabs     panes.Tabs
	roster   roster     // son-akp.5: ids, cached, cachedStates, found, discovery…
	topology topology   // household, askedTopology, askingTopology
	frame    frame      // son-akp.7: width, height, sized, frameRows, screenTop
	help     panes.Modal
	sweep    int        // border light chain
	hintDone bool
	quitting bool

	queue      queueFeature
	favourites favouritesFeature // owns the mark chooser too
	search     searchFeature
	signIn     signInFeature
	settings   settingsFeature
	grouping   groupingFeature
}
```

`appModel` owns routing, layout, the speakers, and the connections *between* features. Features do not know each other.

### 4.2 A feature component

```go
// searchFeature is the search modal and the search it runs.
type searchFeature struct {
	list       listModal // son-akp.4: modal, pulse, hint, help table
	run        int       // generation of the last search asked
	cancel     context.CancelFunc
	classifier musicClassifier
}

func (s searchFeature) update(msg searchMessage, env appEnv) (searchFeature, tea.Cmd, appAsk)
func (s searchFeature) open(env appEnv) (searchFeature, tea.Cmd, appAsk)
func (s searchFeature) resume(env appEnv) (searchFeature, tea.Cmd, appAsk) // after sign-in
func (s searchFeature) reset() searchFeature                               // after sign-out
```

The handler bodies move almost unchanged. `m.search.Content` becomes `s.list.Content`. `m.activeSpeaker()` becomes `env.speaker`. `return m.openSignIn(followSearch)` becomes `ask.signIn = followSearch`.

### 4.3 What a feature reads: appEnv

```go
// appEnv is what a feature may read from the app during one update.
type appEnv struct {
	ctx      context.Context
	speaker  speakerModel // the speaker in view
	speaking bool         // false when no speaker has loaded
	cloud    cloudLink    // client, localID; a copy
	db       *store.Store
}

func (m appModel) env() appEnv
```

Build it fresh for each update. It is a value, so a feature cannot change the app through it. This is like gh-dash's `ProgramContext` and soft-serve's `common.Common`. But those programs share one *pointer* that the root changes. A value removes the aliasing, and costs a few words for each update.

### 4.4 What a feature asks: appAsk

```go
// appAsk is what a feature asks the app to do after its update.
type appAsk struct {
	signIn   *signInFollow // open the sign-in; then follow this
	signedIn *signInFollow // the sign-in worked; tell this feature
	sweep    bool          // start a new border light chain
	localID  string        // the speakers' household id, newly read
}

func (m appModel) follow(cmd tea.Cmd, ask appAsk) (appModel, tea.Cmd)
```

`follow` is the one place that connects features:

```go
if ask.signedIn != nil {
	switch *ask.signedIn {
	case followSearch:
		m.search, c, more = m.search.resume(m.env())
	case followSettings:
		m.settings, c = m.settings.signedIn(m.env())
	}
}
```

**Why a return value and not a message.** Possibility: the feature returns `func() tea.Msg { return openSignInMsg{} }`. Bubble Tea runs commands in goroutines, so:

- the order of commands in a batch is not fixed;
- a frame can render between the two updates;
- every test that checks `m.signIn.IsOpen()` after one `deliver` then needs one more step.

A return value is synchronous and deterministic, and the tests do not change. Keep messages for async results, as now. Keep notices as `panes.NoticeMsg` commands, because that already works.

### 4.5 Routing

Each feature claims its messages with a marker method:

```go
// searchMessage is a message only the search handles.
type searchMessage interface{ forSearch() }

func (searchAskMsg) forSearch()    {}
func (searchDoneMsg) forSearch()   {}
func (searchGuessMsg) forSearch()  {}
// … artistOpenedMsg, albumOpenedMsg, classifierFoundMsg …
```

```go
switch msg := msg.(type) {
case searchMessage:
	return m.routeSearch(msg)
case signInMessage:
	return m.routeSignIn(msg)
// one case for each feature, then the app's own messages
}
```

This is the "one update switch for each feature" possibility that son-akp.3 keeps in view. The number of cases in total stays almost the same, but each case is in its feature's file, next to its message. The compiler checks it. A message without a marker falls through to the tabs, as a missing case does now. A test can check that each message type in a feature file has its marker.

Keys, the mouse and `cursor.BlinkMsg` do not belong to a feature. They go to the top open overlay (4.7).

### 4.6 Shared chains

- **`listModal`** (son-akp.4): `panes.Modal` + `pulse` + `hint` + `help`. It has value methods: `close()`, `notify()`, `restartChain()`, `retireHint()`, `tick(gen)`. This removes `listSlot` and its pointer warning.
- **`borderChain`** for the sign-in and settings: one `gen`, and methods `hintDue`, `notify`, `tick`. Generations are unique in the process, so both can use `listHintDueMsg` and `listHintTickMsg`. This removes 4 message types and 6 methods. The app sends a hint message to each feature that owns a modal. Only the feature whose generation matches does something.

### 4.7 Overlay order

Replace the 6 lists of modals by name with one method:

```go
// overlays are the modals from the top down: the first open one gets the keys.
func (m appModel) overlays() []panes.Modal
```

`growFrame`, `View` (in reverse), `listTakesTheWheel` and the key chain read it. `WindowSizeMsg` calls `setSize` on each feature. First decide open question 3: today the draw order and the key order are different.

### 4.8 Packages

What stops `search` from moving to `internal/ui/search` or `cmd/sonos/search` today:

- `listView[T]`, `listFilter`, `swipeTracker`, the list styles (queuestyles.go) and `listContent` are in `package main`.
- `speakerModel`, `startPlayable`, `startAction`, `resultPlayable` and `storeFavourites` are in `package main`.
- `nextGen`, `notify`, `helpTable` and `hintText` are in `package main`.
- The help guard test (help_test.go) maps each key to a help table by *file name* in `.` and `../../internal/ui/panes`.
- The test helpers (`keyMsgFor`, `flatten`, `plain`, `sizedModel`) are in `package main` tests.

My advice: first move the generic list code (`listView`, `listFilter`, swipe, list styles, maybe the `listModal` chain) to `internal/ui/listview`. That is son-akp.2. Keep the features in `package main` as types in their own files.

The cost: Go does not stop `appModel` from reading `m.search.run` in the same package. The boundary is a rule that review must hold. A subpackage would make the compiler hold it. But it also makes you export many names, copy test helpers, and change the help guard. Do this later, for one feature at a time, when a feature is stable.

### 4.9 Testing

- **The current tests stay.** They drive `appModel.Update` and are the safety net. Most changes are field paths: `m.searchRun` → `m.search.run` (11 uses), `m.classifier` → `m.search.classifier` (6), `m.queueGen` → `m.queue.list.pulse` (about 8), `m.signInGen` → `m.signIn.chain.gen` (2), `m.search.Content` → `searchOf(m)`. Use helpers like the existing `searchOf` and `signInOf`, so the next move changes one helper and not many tests.
- **New unit tests are optional and cheap.** `searchFeature.update(msg, appEnv{cloud: fake})` needs no tabs, no speakers and no size. Check the returned `appAsk` directly. For example: "signed out → `ask.signIn == followSearch`".
- **Determinism.** Each step must return the same commands as before, in the same batch. `flatten` runs every command in a batch, so a step that adds a `tea.Tick` to a batch would make a test sleep. Look for this in review.

### 4.10 Dependencies

No dependency helps here. DI containers (wire, dig, samber/do) solve object graphs. This is message routing in one loop. Bubble Tea has no built-in router, and real programs route by hand as this proposal does. The standard library and the current patterns are enough.

## 5. Mapping to the son-akp beads

| Bead | Verdict | Why |
|---|---|---|
| son-akp.1 Review organisation | **This document is its output** | Close it when you choose tasks from section 6. Its other points (queue.go size, `internal/sonos` mix) are not in this scope. |
| son-akp.2 Move generic components to internal/ui | **Include, narrowed** | Move `listView`, `listFilter`, swipe, list styles. Do not move features yet (4.8). Do it late, after the features stop changing these files. |
| son-akp.3 appMsg routing | **Replace** | `apply(m appModel)` keeps all handlers on `appModel`, which is the opposite of the goal. Use marker interfaces for each feature (4.5). Its rejected possibilities stay rejected. |
| son-akp.4 listModal type | **Include, step 1, extended** | As written, plus value methods (`close`, `notify`, `restartChain`). The three list features then hold one `listModal` each. |
| son-akp.5 Presence group | **Include as written** | Keep `topology` as a separate group, because grouping, `syncPlaying` and events read it, and presence does not. |
| son-akp.6 Cloud and search group | **Replace** | It mixes two owners. Split it: `cloudLink` (`cloud`, `cloudAsked`, `localHousehold`) owned by the app, and the search run (`searchRun`, `searchCancel`) owned by `searchFeature`. Also, the name `cloudState` that the bead suggests already exists in settings.go. |
| son-akp.7 Frame group | **Include as written** | A small, independent step. |
| son-akp.8 samber/hot | **Not related** | No effect on this plan. |
| son-akp.9 Interface checks | **Independent, in progress elsewhere** | Merge it first if possible: it touches the same files. New types in this plan get their checks when they are made. |
| son-akp.10 Modal options and grouping | **Include, changed** | Keep part 1 (`panes.NewModal` options). Replace part 2: features own their modals, and `overlays()` gives one order (4.7). Do it after the feature steps. |

## 6. Migration plan

Each step is one commit, `make vet test` passes, and behaviour does not change. Size: S is less than about 150 changed lines, M is 150–500, and L is more than 500 (mostly moved code).

| # | Step | What moves | What stays | Risk | Size |
|---|---|---|---|---|---|
| 1 | `listModal` type (son-akp.4) | 3 modals + 6 gens into 3 `listModal` fields; `listSlot` and its pointers go | All handlers stay on `appModel` | Low. Mechanical. The tests change gen field paths. | M |
| 2 | `borderChain` for sign-in and settings | `signInGen`, `settingsGen` and their hint and tick messages into one type; reuse the list hint messages | Handlers stay | Low. The `signin_test` hint cases cover it. | S |
| 3 | `cloudLink` group | `cloud`, `cloudAsked`, `localHousehold` into one struct | Handlers stay | Low | S |
| 4 | `appEnv`, `appAsk`, `follow` | The new types only; no use yet | Everything | None. It can join step 5 if you prefer. | S |
| 5 | **Search feature** | searchcontrol.go and searchguess.go handlers become `searchFeature` methods; `searchMessage` markers; one `case` | `lookUpHousehold` stays on the app (speakers start it); sign-out calls `m.search.reset()` | Medium. Cancel semantics, `afterSignIn`, sign-out reset. The 3 search test files cover it well. | L |
| 6 | Sign-in feature | signincontrol.go into `signInFeature`; `afterSignIn` becomes `ask.signedIn` and `follow` | `follow` holds the connection to search and settings | Medium. Key order and blink routing. | M |
| 7 | Settings feature | settingscontrol.go into `settingsFeature` | `cloudState()` reads `env.cloud` | Low | M |
| 8 | Favourites feature, with the mark chooser | favouritescontrol.go, the `marking` modal | `markNowPlaying` (player key `a`) calls into it from the app | Medium. The chooser hovers over the queue. | L |
| 9 | Queue feature | queuecontrol.go handlers; `markPlaying` becomes a method the app calls from `applyEvent` | `refreshQueue` stays a free function (events and speakers use it) | Medium. The largest test set, but it is good. | L |
| 10 | Grouping feature | `openGrouping`, `applyGroupChange`, the `grouping` modal | `env` gets the topology and the tab title | Low | S |
| 11 | Presence, topology and frame groups (son-akp.5, .7) | State only | Handlers | Low | S each |
| 12 | `overlays()` and modal options (son-akp.10) | One order for draw, keys, size, frame height | — | Medium. Decide open question 3 first. Add a test for each order. | M |
| 13 | `internal/ui/listview` (son-akp.2) | Generic list code | Features | Low to medium. Export names; the help guard paths. | M |

Steps 1–3 and 11 are independent, and each is worth doing alone. You can stop after step 7: then search, sign-in and settings, the part that grows fastest, are done.

## 7. Risks and trade-offs

- **More plumbing, not less code.** The total line count goes up a little (`appEnv`, `appAsk`, markers, `route*` glue). The gain is location: each feature is in one place, and `appModel` drops from 46 to about 16 fields.
- **"Where is this message handled?"** now means "which feature has the marker". This is one more jump than one switch, but the marker is next to the message type.
- **No compiler-enforced privacy** in `package main` (4.8).
- **`appAsk` can grow into a second message bus.** Keep it to requests that must be synchronous. Today that is 3–4 fields. If it grows past about 6, think again.
- **Test churn.** About 100 mechanical edits over the steps. Add accessors early to make each later step smaller.
- **Conflicts with work in progress.** son-akp.9 runs in another worktree and touches many of the same files. Merge it before step 1.
- **Do not change behaviour while refactoring.** If you find bugs, such as problem 7, put them in their own beads and commits.

## 8. Open questions

1. **Asks: return value or messages?** I recommend a return value (4.4). Do you agree, or do you prefer messages for uniformity at the cost of extra test steps?
2. **Packages:** keep the features in `package main` for now (my advice), or go to subpackages for compiler-enforced boundaries?
3. **Draw order and key order:** help draws over the sign-in, but the sign-in takes keys first. Is this intended? Which order should `overlays()` use?
4. **The household id:** carry it back through `appAsk.localID` (pure, my advice), or keep it in a small mutex-guarded holder in `cloudLink`, so 4 message types lose their `localID` field?
5. **Border chains:** can the sign-in and settings reuse `listHintDueMsg` and `listHintTickMsg`, or do you want their own names for clarity?
6. **Tests:** can tests read feature internals (`m.search.run`), or only through helpers?
7. **Order:** this plan does son-akp.4 before search, because `searchFeature` holds a `listModal`. Is that acceptable, or must search come first?
8. **Speakers:** I advise that the tabs, presence and topology stay `appModel`'s core as state groups, not a feature component. Do you agree?
