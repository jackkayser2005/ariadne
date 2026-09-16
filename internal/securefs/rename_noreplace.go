package securefs

import "errors"

var errAtomicRenameUnsupported = errors.New("atomic no-replace directory publication is unsupported on this platform")
