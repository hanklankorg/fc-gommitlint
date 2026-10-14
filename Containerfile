# SPDX-FileCopyrightText: The Gommitlint Authors
#
# SPDX-License-Identifier: CC0-1.0

# The binary is a dynamically linked PIE: it needs the glibc loader that
# distroless/base carries and distroless/static does not. The image has no git,
# so it validates SHA-1 repositories only and is published as
# <version>-distroless. The default image is Containerfile.alpine, which
# installs git and reads SHA-256 repositories too.
ARG BASE_IMAGE=gcr.io/distroless/base-debian13:nonroot@sha256:0896741ba5bafd3ac87ea025a5f578952f2d238ddc3614cb368acc983a687aa2
FROM ${BASE_IMAGE}

# Directory holding the binary, with a trailing slash. The release workflow
# passes "dist/gommitlint-pie_linux_<arch>_<variant>/"; `just build-image`
# passes its staging directory under bin/. .containerignore admits only those.
ARG DIRPATH
COPY --chmod=755 ${DIRPATH}gommitlint /usr/local/bin/gommitlint

ENTRYPOINT ["/usr/local/bin/gommitlint"]
