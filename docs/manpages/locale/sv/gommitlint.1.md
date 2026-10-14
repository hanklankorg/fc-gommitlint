# GOMMITLINT 1 "Användarkommandon"

## NAMN

gommitlint - git-incheckningslinter

## SYNOPSIS

**gommitlint** [*FLAGGA*...] *KOMMANDO* [*argument*...]
**gommitlint** [*FLAGGA*...] **validate** [*FLAGGA*...]
**gommitlint** [*FLAGGA*...] **doctor**
**gommitlint** [*FLAGGA*...] **config** {**show**|**check**|**init**|**schema**|**sync**}
**gommitlint** [*FLAGGA*...] **hook** {**install**|**remove**|**status**} [*FLAGGA*...]

## BESKRIVNING

**gommitlint** validerar Git-incheckningsmeddelanden mot konfigurerade regler.
Det stöder konventionellt incheckningsformat, kontroller av ämnesrad och brödtext, JIRA-referenser, stavningskontroll, policy för författarens e-postadress samt kontroll av signaturens närvaro, format, typ och nyckelinventering. Det verifierar eller auktoriserar inte en inchecknings signerare kryptografiskt.

Som standard validerar **gommitlint** HEAD-incheckningen.
Använd **--base-branch** för att validera alla incheckningar i en funktionsgren, eller **--count** för att validera flera av de senaste incheckningarna.

En *FIL* med värdet "-" betyder standardindata när **--message-file** används.
Om inget mål anges validerar **gommitlint** HEAD-incheckningen.
Om **--base-branch** anges validerar **gommitlint** alla incheckningar från basgrenen till HEAD.

Som standard slås inbyggda värden, globala inställningar under *$XDG_CONFIG_HOME/gommitlint/settings.{yaml,yml,toml}* och projektets *.gommitlint.{yaml,yml,toml}* samman. **--config** väljer ett separat läge med en explicit fil och **--ignore-config** använder endast standardvärden. Miljövariabler påverkar sökvägar och särskilda körningsflaggor, inte ett allmänt konfigurationslager.

**gommitlint** validerar incheckningar med 10 konfigurerbara regler: **branchahead**, **commitbody**, **conventional**, **cryptosignature**, **identity**, **jirareference**, **linearhistory**, **signoff**, **spell**, och **subject**.
Som standard är 7 regler aktiva (inaktiverade: identity, jirareference, linearhistory).

**gommitlint** har fyra huvudkommandon:

- **validate** - Validera incheckningsmeddelanden mot regler
- **doctor** - Inspektera konfiguration, krokstatus och effektiv policy
- **config** - Hantera konfiguration (visa effektiv konfiguration, generera mallar)
- **hook** - Hantera Git-krokar (installera, ta bort, kontrollera status)

Valideringsresultat skrivs till standard ut; loggar, fel och diagnostik går till standard fel. Se **STANDARD UT OCH STANDARD FEL** och **EXITSTATUS** för detaljer.

## GLOBALA FLAGGOR

**--help**, **-h**
: Visa hjälp och avsluta.

**--version**
: Visa versionsinformation och avsluta.

**--config** *FIL*
: Läs in inbyggda standardvärden samt *FIL* och dess importer. Läs inte in automatiskt upptäckt global konfiguration eller projektkonfiguration.

**--ignore-config**
: Använd inbyggda standardvärden och ignorera alla konfigurationsfiler.

**--repo-path** *SÖKVÄG*
: Använd Git-kodförråd vid *SÖKVÄG* (standard: aktuell katalog).

**--color** *LÄGE*
: Styr färgad utdata: auto, always, never (standard: auto).

**--theme** *NAMN*
: Välj utdatatema. Kör **gommitlint config show** för att se standardvärdet och de värden som stöds.

**--quiet**, **-q**
: Undertryck all utdata utom fel.

**--log-level** *NIVÅ*
: Ställ in loggningsnivå: error, warn, info, debug, trace (standard: info).

**--allow-coredumps**
: Tillåt kärndumpar vid krasch. Som standard sätter gommitlint RLIMIT_CORE till noll (och på Linux PR_SET_DUMPABLE=0) vid uppstart, så att en krasch med känsligt innehåll i minnet inte hamnar på disk. Använd flaggan endast när du diagnostiserar en gommitlint-krasch som kräver en kärnfil. Kan också anges via **GOMMITLINT_ALLOW_COREDUMPS=1** i miljön.

## VALIDATE-FLAGGOR

**--message-file** *FIL*, **-f** *FIL*
: Läs incheckningsmeddelande från *FIL* i stället för från Git (använd - för stdin).

**--base-branch** *GREN*, **-b** *GREN*
: Validera alla incheckningar från *GREN* till HEAD. Accepterar en kommaseparerad lista som reserv (t.ex. main,master,develop); den första befintliga grenen används. Om ingen finns hoppas grenberoende validering över med en varning.

**--range** *INTERVALL*
: Validera incheckningar i git-*INTERVALL* (t.ex. main..feature).

**--ref** *REF*, **-r** *REF*
: Validera specifik git-*REF* (standard: HEAD).

**--count** *N*, **-n** *N*
: Validera de senaste *N* incheckningarna från HEAD (standard: 1).

**--format** *FORMAT*
: Utdataformat: text, plain, json, sarif, github, gitlab, forgejo, table (standard: text).

**--verbose**, **-v**
: Öka detaljnivån i utdata (**-v**/--verbose eller **-vv**, inte båda).

**--rule** *REGLER*
: Validera endast specifika *REGLER* (kommaseparerade, t.ex. conventional,signoff).

**--exclude-rule** *REGLER*
: Uteslut specifika *REGLER* från validering (kommaseparerade, t.ex. cryptosignature).

**--ignore-repo-rules**
: Kör endast regler för incheckningsmeddelanden, ignorera alla kodförrådsregler.

**--no-ci-detect**
: Inaktivera automatisk identifiering av CI-miljö. Som standard upptäcker **gommitlint** GitHub Actions, Forgejo/Gitea Actions och GitLab CI för att automatiskt sätta **--base-branch** och **--format**. Denna flagga inaktiverar det beteendet. Kan även sättas via miljövariabeln **GOMMITLINT_NO_CI_DETECT**.

**--rule-help** *REGEL*
: Visa detaljerad hjälp för specifik *REGEL* och avsluta.

**--report-file** *FIL*
: Skriv valideringsresultat till *FIL* (använd - för stdout).

## HOOK-FLAGGOR

**--force**, **-f**
: Skriv över befintlig krok om den finns (kommandot hook install).

**--global**
: Installera/ta bort krokar under *$XDG_CONFIG_HOME/git/git-hooks/*, med standardvärdet *~/.config/git/git-hooks/* när **XDG_CONFIG_HOME** inte är satt, och hantera global git config core.hooksPath. Installationen vägrar skriva över ett annat värde om inte **--force** sparar och ersätter det; borttagning återställer värdet när det är säkert.

## CONFIG-FLAGGOR

**--format** *FORMAT*
: Utdataformat för config show: text, json, yaml (standard: text).

**--view** *VY*
: Välj detaljnivå för textutdata från **config show** och **config check**: compact, verbose (standard: verbose).

**--source** *ID*
: För **config sync**, synkronisera endast den managed source som har *ID*.

**--dry-run**
: För **config sync**, hämta och verifiera managed sources utan att skriva filer.

**--check**
: För **config sync**, misslyckas om någon lokalt vendorad managed source är inaktuell.

**--timeout** *DURATION*
: För **config sync**, begränsa varje hämtning av en managed source (standard 10s).

**--compact**
: För **config schema**, skriv ut JSON på en rad i stället för indenterat (standard: indenterat).

**--preset** *PRESET*
: För **config init**, skriv en liten konfiguration som utökar en inbyggd preset: minimal, conventional, ambitious eller secure.

## STANDARD UT OCH STANDARD FEL

**gommitlint** separerar resultat från diagnostik så att utdata kan kedjas och tolkas på ett pålitligt sätt.

*Standard ut* innehåller enbart valideringsresultat — rapporten vald med **--format** (text, plain, json, sarif, github, gitlab, forgejo, table). Med **--format=json** är standard ut ett enda JSON-dokument lämpligt för `jq`, CI-konsumenter eller omdirigering. Använd **--report-file** *FIL* för att skriva resultat till *FIL* i stället; ange **--report-file -** för att uttryckligen tvinga resultat till standard ut.

*Standard fel* innehåller allt annat: loggposter (styrs av **--log-level**), varningar, användningsfel och diagnostikmeddelanden från krokar eller konfigurationsinläsning. Denna ström är inte en del av det maskinläsbara kontraktet — innehåll och format kan ändras mellan utgåvor.

Denna separation innebär att `gommitlint validate --format=json 2>/dev/null | jq .` ger ren JSON oavsett loggnivå, och omdirigering av standard fel döljer inte valideringsutdata.

**--quiet** undertrycker icke-fel-utdata på båda strömmarna; fel går fortfarande till standard fel. **--log-level** påverkar endast standard fel; den ändrar inte vad som visas på standard ut.

## EXITSTATUS

**gommitlint** följer POSIX-konventioner för `0`, `1`, `2` och signalhärledda koder, samt BSD *sysexits.h*-konventioner (64–78) för felklasser där en specifik kategori finns. CI-skript som bara bryr sig om pass/fail bör kontrollera `exit == 0` eller `exit != 0`; skript som vill skilja "incheckningen är fel" från "gommitlint kunde inte köra" bör behandla `1` särskilt och se övriga koder som miljöproblem.

**0**
: Inga överträdelser hittades, alla incheckningar är giltiga.

**1**
: Valideringsregler bröts i incheckningsmeddelanden eller kodförråd.

**2**
: Felaktig användning — ogiltiga flaggor, okänt underkommando eller avbrott utan associerad signal. Reserverad för situationer som inte passar någon *sysexits.h*-kategori.

**64**
: Användningsfel (*EX_USAGE*) när **gommitlint** körs utan underkommando. Ett kort användningskort skrivs till standard fel.

**65**
: Felaktigt indata (*EX_DATAERR*). Användaren angav en referens, ett intervall eller ett antal som är syntaktiskt ogiltigt eller utanför tillåtna gränser.

**66**
: Saknat indata (*EX_NOINPUT*). En referens, ett intervall eller en konfigurationssökväg som användaren angav på kommandoraden kan inte hittas (incheckning okänd, --config pekar på en fil som inte finns).

**69**
: Extern tjänst otillgänglig (*EX_UNAVAILABLE*). Den Git-miljö som gommitlint förlitar sig på saknas eller misslyckades (arbetsmappen är inte ett Git-kodförråd, Git-underprocess misslyckades).

**70**
: Internt programfel (*EX_SOFTWARE*). Ett inbäddat delsystem misslyckades av skäl som varken beror på användarindata eller konfiguration (t.ex. stavningskontrollen kraschade). Ohanterade panics fångas av en toppnivågräns. Standard fel får ett säkert meddelande, sökvägen till den privata rapporten och en förifylld ärendelänk. Det begränsade panic-värdet och stackspårningen skrivs till en fil med läge `0600` under **$XDG_STATE_HOME/gommitlint/crash** (normalt **~/.local/state/gommitlint/crash**).

**77**
: Behörighets- eller nyckelpolicyfel (*EX_NOPERM*). Signeringsnyckelkonfigurationen är ofullständig — nyckelkatalogen saknas eller kan inte läsas, inga godtagbara nycklar är konfigurerade, eller nyckelinläsningen misslyckades.

**78**
: Konfigurationsfel (*EX_CONFIG*). Ogiltig eller felaktig konfigurationsfil; kodförrådsregler konfigurerade utan att vara i ett Git-kodförråd.

**130**
: Avbruten av SIGINT (Ctrl-C). `128 + 2` enligt POSIX-konvention.

**131**
: Avbruten av SIGQUIT (Ctrl-\). `128 + 3`.

**143**
: Avbruten av SIGTERM (kill-kommando). `128 + 15`.

## MILJÖ

**NO_COLOR**
: Inaktiverar färgad utdata om variabeln är satt.

**GOMMITLINT_DEBUG**
: Aktiverar felsökningsloggning i Git-krokar om variabeln är satt (sätter --log-level=debug).

**GOMMITLINT_NO_CI_DETECT**
: Inaktiverar automatisk identifiering av CI-miljö om variabeln är satt. Motsvarar **--no-ci-detect**.

**GOMMITLINT_ALLOW_COREDUMPS**
: Tillåter kärndumpar om variabeln är satt till ett sant värde. Motsvarar **--allow-coredumps**. Avsedd för utvecklare som felsöker gommitlint-krascher — värdmaskinens RLIMIT_CORE-policy lämnas oförändrad för denna körning.

**XDG_CONFIG_HOME**
: Katalog för användarkonfiguration (standard: ~/.config).
Konfiguration letas efter i $XDG_CONFIG_HOME/gommitlint/settings.yaml, settings.yml och settings.toml.

## FILER

**.gommitlint.yaml**
: Standardkonfigurationsfil i aktuell katalog.

**.gommitlint.yml**
: Alternativt konfigurationsfilnamn.

**.gommitlint.toml**
: Konfigurationsfil i TOML-format.

**$XDG_CONFIG_HOME/gommitlint/settings.yaml**
: Användarkonfigurationsfil (XDG Base Directory-specifikation).

**$XDG_CONFIG_HOME/gommitlint/settings.yml**
: Användarkonfigurationsfil (YAML).

**$XDG_CONFIG_HOME/gommitlint/settings.toml**
: Användarkonfigurationsfil (TOML).

**<gemensam-git-katalog>/hooks/commit-msg**
: Git-krok för meddelandevalidering, installerad av **hook install**. I ett vanligt kodförråd är detta *.git/hooks/commit-msg*; länkade arbetskataloger delar huvudförrådets gemensamma krokkatalog. Lokal installation vägrar när core.hooksPath omdirigerar krokar.

**<gemensam-git-katalog>/hooks/pre-push**
: Git-krok för konfigurerad signaturpolicy, med samma sökvägsupplösning som commit-msg.

## EXEMPEL

Validera den aktuella HEAD-incheckningen:

    gommitlint validate

Validera alla incheckningar i aktuell funktionsgren mot main:

    gommitlint validate --base-branch=main

Validera en specifik incheckning:

    gommitlint validate --ref=HEAD~1

Validera ett incheckningsmeddelande från en fil:

    gommitlint validate --message-file=commit-msg.txt

Validera ett intervall av incheckningar:

    gommitlint validate --range=main..feature

Validera de senaste 5 incheckningarna med utförlig utdata:

    gommitlint validate --count=5 --verbose

Validera endast Conventional Commits-format:

    gommitlint validate --rule=conventional

Uteslut kontroller av signaturmetadata och nyckelinventering:

    gommitlint validate --exclude-rule=cryptosignature

Installera commit-msg- och pre-push-krokar:

    gommitlint hook install

Installera krokar globalt (alla kodförråd):

    gommitlint hook install --global

Skapa en konfigurationsfil i nuvarande katalog:

    gommitlint config init

    gommitlint config init --preset minimal    # skapa en liten preset-konfiguration

    gommitlint config init --force            # skriv över befintlig .gommitlint.yaml

    gommitlint config init --output custom.yaml

    gommitlint config init --stdout > .gommitlint.yaml   # skriv till stdout (gammal pipe-form)

Visa aktuell effektiv konfiguration:

    gommitlint config show --format=yaml
    gommitlint config show --view=compact

Kontrollera konfigurationshälsan:

    gommitlint config check
    gommitlint config check --format=json
    gommitlint config check --view=compact

Synkronisera hanterad teamkonfiguration:

    gommitlint config sync

Kontrollera om lokalt hämtad hanterad konfiguration är uppdaterad:

    gommitlint config sync --check

Skriv ut JSON-schemat för konfigurationsfilen:

    gommitlint config schema > .gommitlint.schema.json
    gommitlint config schema --compact

Kontrollera aktuell krokstatus:

    gommitlint hook status

Utdata i JSON för CI/CD-pipelines:

    gommitlint validate --format=json

Validera i CI med autoidentifiering (upptäcker plattform, format och basgren):

    gommitlint validate

Utdata i GitHub Actions-format:

    gommitlint validate --format=github

## KONFIGURATION

**gommitlint** använder YAML- eller TOML-konfiguration med en "gommitlint"-rotnyckel. Inläsningen har tre lägen.

Exempelkonfiguration (minimal):

    gommitlint:
      message:
        subject:
          max_length: 100
      rules:
        disabled: ["jirareference"]

Regeln **branchahead** utvärderas bara när **--base-branch** anges, och använder då den grenen för jämförelse.

**Upptäcktslägets sammanslagningsordning** (senare källor skriver över tidigare):

1. Inbyggda standardvärden
2. Global konfiguration: *$XDG_CONFIG_HOME/gommitlint/settings.yaml* (eller *.yml*, *.toml*)
3. Projektkonfiguration: *.gommitlint.yaml* i aktuell katalog (eller *.yml*, *.toml*)
När både global och projektlokal konfiguration finns slås de samman. Projektinställningar skriver
över globala inställningar. Använd **gommitlint config show** för att se den effektiva
sammanslagna konfigurationen och dess källor.

**Explicit filläge:** **--config** läser in inbyggda standardvärden samt den angivna filen och dess importer. Upptäckt global konfiguration och projektkonfiguration läses inte in.

**Endast standardvärden:** **--ignore-config** läser inte in några konfigurationsfiler och kan inte kombineras med **--config**.

Den kanoniska standardkonfigurationen och referensen finns i `docs/modules/ROOT/pages/reference.adoc`.

Lokala konfigurationsfiler kan ärva från andra lokala konfigurationsfiler med `extends`.
Ärvda filer laddas först och den aktuella filens inställningar har företräde.

Fjärrstyrd teamkonfiguration kan deklareras med `managed_sources` och synkroniseras explicit med
**gommitlint config sync**. Hanterade källor hämtas över HTTPS, verifieras med en konfigurerad
SHA-256-kontrollsumma och sparas lokalt. Vanlig validering gör aldrig nätverksanrop.
URL:er för managed_sources kan anges direkt som HTTPS-URL:er eller som `${ENV_VAR}`-referenser som
bara löses upp under **gommitlint config sync**.

För felsökning av delad konfiguration kan du inspektera den effektiva konfigurationen med
**gommitlint config show --format=json**, köra **gommitlint config check**, och använda
**gommitlint config sync --check** för att verifiera om lokalt hämtade managed_sources är aktuella.
För att uppdatera en enskild källa direkt, kör **gommitlint config sync --source <id>**.

**gommitlint config show** innehåller också konfigurationskällornas graf och vald härkomst för att
hjälpa till att förklara var effektiva inställningar kommer ifrån, inklusive explicita regel-
överstyrningar. Inspektionen av managed_sources visar också om en källa är direkt eller miljö-
baserad. När en inställning skriver över ett tidigare värde visar härkomsten också den överstyrda
källan och värdet. För miljöbaserade managed_sources visar utdata också den konfigurerade käll-
strängen och om den senaste synkroniseringen läste källan via miljön. Härkomsten förklarar också
varför viktiga värdesättningar och regelstatusar är aktiva.

**gommitlint config check** innehåller både grupperade problemrapporter och ett sammanfattningsblock
för managed_sources, så att delade policyer med flera källor blir lättare att felsöka, tillsammans
med ett rekommenderat nästa kommando när det finns. Maskinläsbar utdata innehåller samma
rekommendation som `recommended_next_command`.

## BUGGAR

### Rapportera buggar

Rapportera buggar på: <https://codefloe.com/itiquette/gommitlint/issues>

### Kända problem

I stora kodförråd med många incheckningar kan valideringen ta längre tid. Använd **--count** eller
**--range** för att begränsa omfattningen för bättre prestanda.

## SE ÄVEN

**git**(1), **git-commit**(1), **git-log**(1), **git-hook**(5)

Fullständig dokumentation: <https://codefloe.com/itiquette/gommitlint>

Conventional Commits-specifikationen: <https://conventionalcommits.org>

## UPPHOVSRÄTT

Copyright © 2025 The Gommitlint Authors. Dubbellicensierad: välj antingen EUPL-1.2 <https://eupl.eu/1.2/en/> eller GPL-3.0-or-later <https://www.gnu.org/licenses/gpl-3.0.html>.

Detta är fri programvara: du får ändra och distribuera den vidare. Det finns INGEN GARANTI i den
utsträckning som lagen tillåter.
