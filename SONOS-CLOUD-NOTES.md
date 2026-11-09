# Cloud notes

Findings about the Sonos cloud, as the Sonos web app (play.sonos.com) uses
it. `SONOS-NOTES.md` holds the findings about the speakers on the local
network; this file holds what needs the internet and a Sonos account.

Everything here was measured with `experiments/sonosweb` (`make sonos-web`) in
November 2026. These are unofficial, undocumented endpoints of Sonos's web
app. They can change or stop at any time, and to use them from a program is
probably outside Sonos's terms. By the README's principles, a feature built on
them must be optional: the program must work fully without them.

## Signing in

The web app is a Next.js app that uses NextAuth. NextAuth has one provider,
`okta`, and the Okta organisation is Sonos's own, at `login.sonos.com`.

The sign-in needs no JavaScript and no browser. A program with a cookie jar
does it in five steps:

1. `GET https://play.sonos.com/` redirects (307) through `/api/auth/signin`
   and `/login` to `/en-us/login`. That page has no form: in the browser, a
   script starts the sign-in.
2. `GET /api/auth/csrf` gives a `csrfToken` and sets the
   `__Host-next-auth.csrf-token` cookie.
3. `POST /api/auth/signin/okta`, form encoded, with `csrfToken`,
   `callbackUrl` and `json=true`, answers with JSON: `{"url": ...}`, the Okta
   authorize address. It also sets the PKCE and state cookies that NextAuth
   checks on the way back.
4. `GET` that address. Okta answers with its classic sign-in widget page. The
   page's script holds `oktaData.signIn.stateToken`, written with `\xNN`
   escapes. The page has `authn: true` and `isBotProtectionEnabled: false`,
   and no captcha.
5. `POST https://login.sonos.com/api/v1/authn`, JSON, with `username`,
   `password` and `stateToken`. A good sign-in answers `status: SUCCESS` with
   `_links.next.href`. A `GET` of that address redirects through Okta to
   `play.sonos.com/api/auth/callback/okta` with the code, and NextAuth sets
   the session cookie there.

The account used had no second factor. With one, step 5 answers a status
that starts with `MFA_` and lists the factors; the experiment stops there.

The authorize request, for reference:

| Field | Value |
|---|---|
| Address | `https://login.sonos.com/oauth2/aus91jbc1Ay0rx8hv4x6/v1/authorize` |
| `client_id` | `6dd0737e-2268-4263-bfb7-328ac7c726a2` |
| Flow | authorization code with PKCE (`S256`) |
| Scopes | `openid playback-control-all identity-read-basic offline_access content-directory-all muse-events authz-internal-all hh-config hh-config-admin` |

## The session

The session is one cookie, `__Secure-next-auth.session-token`. NextAuth
splits a large session into numbered parts (`.0`, `.1`); this one was not
split. It is a JWE that only the web app's server can decrypt, so a program
cannot read the Sonos tokens inside it. It sends the cookie and lets the web
app's backend call the Sonos cloud.

`GET /api/auth/session` says what the session holds:

- `expires` and `sessionExp`: when the session ends.
- `user`: the fields `sonosId`, `account` and `ip`.
- `accessTokenStatus`: `{"status": "present", "expiration": <ms>}`, the
  Sonos access token inside the session.
- `version`: 3.

**The session renews when it is used.** `expires` is always 10 days after
the last request. Three runs 12 minutes apart moved it 12 minutes forward.
A session not used for 10 days ends, and the next use must sign in again.

**The access token inside lasts 24 hours.** Its `expiration` was 24 hours
after the sign-in, and it did not move when the session was used. The sign-in
asks for `offline_access`, so the web app's backend probably renews it with a
refresh token. This is not measured yet: search once more than 24 hours after
a sign-in, and see if it works and if `expiration` moved.

The experiment keeps the cookie in `~/.config/sonos-sandbox/web-session`
(mode 0600), outside the repository. The cookie is a sign-in to the Sonos
account: do not print it, log it or commit it.

## The household id

A search address holds the household id. The sign-in pages, the session
(`/api/auth/session`) and the signed-in page (`/en-us/web-app`) do not have
it. The web app gets it from Sonos's Control API, over a websocket:

```
wss://api.ws.sonos.com/websocket?ticket=<uuid>
```

The ticket comes from the web app's backend, with only the session cookie:

```
GET https://play.sonos.com/api/mfe
{"ticket": "<uuid>", "fingerprint": "<uuid>"}
```

Each request gives a new ticket. The websocket needs the subprotocol
`v1.api.smartspeaker.audio`, as the speakers' own Control API does: without
it, the server answers 200 and does not open the websocket. The experiment
also sends `Origin: https://play.sonos.com`; if that is necessary is not
tested. The `fingerprint` is not used.

Each message is a JSON array of two objects: a header and a body. The web
app's first message:

```json
[{"command": "getHouseholds", "namespace": "households", "corrId": "<uuid>"},
 {"connectedOnly": "true"}]
```

The answer has the same `corrId`:

```json
[{"apiVersion": 1, "corrId": "<uuid>", "response": "getHouseholds", "success": true,
  "namespace": "households", "execTimeMs": 80, "type": "households", "receivedAt": 1794256158270},
 {"households": [{"id": "Sonos_…", "swVersion": "97.2-81220", "ownerLuid": "…"}]}]
```

The id has the form `Sonos_`, 26 letters and digits, a dot and 20 letters and
digits (53 characters; measured on one household). The speakers give the same
id on the local network, through DeviceProperties `GetHouseholdID`.

## Search

Every search request needs only the session cookie and the household id. The service id is the one in the `SA_RINCON{id}_` desc of a DIDL:
Apple Music is 52231 (204 × 256 + 7). The account is the `sn=` of a service
URI.

### Search across services

```
GET /api/content/v1/households/{household}/search?query=tom+waits&services=52231,68615
```

`services` takes several ids. The answer has one entry for each service in
`services[]`, each with `serviceId`, `accountId`, `info` (`count`,
`offset`, `pageSize`), `errors` and `resources[]`. All types are mixed in one
list: a search for "tom waits" gave 15 artists, most of them not related
(Sia, John Lennon), and 5 tracks. It gave no albums or playlists.

### Search one service

```
GET /api/content/v1/households/{household}/services/52231/accounts/2/search?query=david+bowie&count=50
```

This is the better search. The answer groups the results by type, in the
top-level keys `ARTISTS`, `TRACKS`, `ALBUMS`, `PLAYLISTS` and `PROGRAMS`.
Each has `info` (`count`, `offset`, `pageSize`) and `resources[]`.
`resourceOrder` gives the order the web app shows them in. A search for
"david bowie" gave 2 artists, 15 tracks, 14 albums, 2 playlists and 15
programs, all related.

### Results

Each resource has:

- `id`: `objectId` (the service's id, such as `album:1676286487`),
  `serviceId`, `accountId`, `serviceName` and `serviceNameId`
  (`apple-music`).
- `name`, `type`, `playable`, `explicit`, `images[]` (`url`) and `actions`
  (`ACTION_PLAY`, `ACTION_FAVORITES`).
- `defaults`: base64 of a small JSON summary with `name`, `artist`,
  `artistId`, `summary`, `imageUrl` and more, by type.

By type:

| Type | `objectId` | What else it has |
|---|---|---|
| `ARTIST` | `artist:551695` | not playable |
| `ALBUM` | `album:1676286487` | `artists[]` with name and id; `summary` (`Rock - 2016`); `enumerable` |
| `TRACK` | `song:1347894092` | `artists[]`; `container` (its album, with its own `artists[]`); `ordinal`; `durationMs` |
| `PLAYLIST` | `playlist:pl.606df…` | `owner`; `readOnly`; `renameable`; `userContent`; `summary` |
| `PROGRAM` | `radio:ra.551695` | an Apple Music station, such as "David Bowie & Similar Artists" |

For this program, two findings are important:

- **An album's artist is in the result.** That is the field the speakers do
  not give when the Sonos app starts an album.
- **A track's album is in the result**: `container`, and `containerId`,
  `containerName` and `containerType` in its `defaults`.

The ids look like the ones in the speakers' URIs: `album:1440750286` is in
`x-rincon-cpcontainer:00040000album%3a1440750286?sid=204…`. That a search
result can be played through the speakers is not tested yet.

### What the household id is for

The household id only tells the cloud which household's linked accounts to
search with. The ids in the results are not specific to a household:

- **Catalogue ids are the same everywhere.** `album:…`, `song:…`, `artist:…`
  and `playlist:pl.…` are Apple Music's own catalogue ids. They name the same
  item in every household.
- **Library ids are personal.** Ids from a person's own library, such as
  `libraryplaylist:p.…`, and user playlists (`userContent: true`) belong to
  one Apple Music account. Another account cannot play them.
- **The account number is the household's.** `accountId` in a search address
  and `sn=` in a playable URI say which of the household's linked accounts
  to use. The same Apple Music account can have another number in another
  household. To play a result, use the local household's `sn`, not a fixed
  number.
- **Results follow the account's country.** The search uses the linked
  account's Apple Music storefront. Catalogue ids are mostly the same across
  countries, but what is available, and what the search returns, can differ.
