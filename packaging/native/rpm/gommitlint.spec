# SPDX-FileCopyrightText: The Gommitlint Authors
# SPDX-License-Identifier: CC0-1.0
#
# RPM spec file for gommitlint
# For building from source for Fedora/RHEL/SUSE repositories

%global forgeurl    https://codefloe.com/itiquette/gommitlint
%global commit      %{?commit}%{!?commit:HEAD}

# Go binaries are statically linked; no debug source files to extract
%global debug_package %{nil}

Name:           gommitlint
Version:        0.9.11
Release:        1%{?dist}
Summary:        Git commit message linter and validator
License:        EUPL-1.2 OR GPL-3.0-or-later
URL:            %{forgeurl}
Source0:        %{forgeurl}/archive/v%{version}.tar.gz#/%{name}-%{version}.tar.gz

BuildRequires:  golang >= 1.23
BuildRequires:  git
BuildRequires:  go-md2man
BuildRequires:  gettext
Requires:       git
Recommends:     ca-certificates

%description
Gommitlint validates git commit messages against configurable rules
including Conventional Commits format, subject/body length limits,
spell checking, imperative mood, and GPG/SSH signature policy checks.

It integrates with git hooks and CI/CD pipelines to enforce consistent
commit message standards across development teams.

Features:
 * Conventional Commits validation
 * Configurable subject/body length limits
 * Spell checking with custom dictionaries
 * Imperative mood enforcement
 * GPG/SSH signature presence, format, type, and key-inventory policy
 * Git hook integration

%prep
%autosetup -n %{name}

%build
export CGO_ENABLED=0
export GOFLAGS="-trimpath -buildvcs=false"
export LDFLAGS="-w -X 'main.version=%{version}' -X 'main.commit=%{commit}' -X 'main.date=${SOURCE_DATE_EPOCH:-$(date -u +%%s)}'"

# Build main binary (PIE for security hardening, matches goreleaser build)
go build -buildmode=pie -tags "netgo osusergo" -ldflags "${LDFLAGS}" -o %{name} ./cmd/gommitlint


# Generate man pages
./scripts/docs/manpage.sh

# Generate shell completions (BINARY_PATH points to the just-built binary)
BINARY_PATH="./%{name}" ./scripts/docs/completions.sh

# Generate translations
./%{name} translations compile

%install
# Binaries
install -D -m 0755 %{name} %{buildroot}%{_bindir}/%{name}

# Man pages
install -D -m 0644 generated/manpages/%{name}.1.gz %{buildroot}%{_mandir}/man1/%{name}.1.gz
install -D -m 0644 generated/manpages/sv/man1/%{name}.1.gz %{buildroot}%{_mandir}/sv/man1/%{name}.1.gz

# Shell completions
install -D -m 0644 generated/completions/%{name}.bash %{buildroot}%{_datadir}/bash-completion/completions/%{name}
install -D -m 0644 generated/completions/%{name}.zsh %{buildroot}%{_datadir}/zsh/site-functions/_%{name}
install -D -m 0644 generated/completions/%{name}.fish %{buildroot}%{_datadir}/fish/vendor_completions.d/%{name}.fish

# Translations
install -D -m 0644 generated/locale/en/LC_MESSAGES/%{name}.mo %{buildroot}%{_datadir}/locale/en/LC_MESSAGES/%{name}.mo
install -D -m 0644 generated/locale/sv/LC_MESSAGES/%{name}.mo %{buildroot}%{_datadir}/locale/sv/LC_MESSAGES/%{name}.mo

%find_lang %{name}

%check
go test -v ./...

%files -f %{name}.lang
%license LICENSES/*
%doc README.adoc SECURITY.md CHANGELOG.md docs/
%{_bindir}/%{name}
%{_mandir}/man1/%{name}.1.gz
%{_mandir}/sv/man1/%{name}.1.gz
%{_datadir}/bash-completion/completions/%{name}
%{_datadir}/zsh/site-functions/_%{name}
%{_datadir}/fish/vendor_completions.d/%{name}.fish

%changelog
* Thu Apr 16 2026 Josef Andersson <janderssonse@proton.me> - 0.9.11-1
- New upstream release 0.9.11

* Mon Feb 09 2026 Josef Andersson <janderssonse@proton.me> - 0.9.1-1
- New upstream release 0.9.1

* Thu Jan 29 2026 Josef Andersson <janderssonse@proton.me> - 0.9.0-1
- Initial RPM spec for source builds
- See CHANGELOG.md for detailed release notes
