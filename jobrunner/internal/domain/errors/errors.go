package errors


type CheckerException struct {
    message string
}

func (e *CheckerException) Error() string {
    return e.message
}

func NewCheckerException(msg string) *CheckerException {
    return &CheckerException{message: msg}
}

type CheckerValidationError struct {
    CheckerException
}

func NewCheckerValidationError(msg string) *CheckerValidationError {
    return &CheckerValidationError{
        CheckerException: CheckerException{message: msg},
    }
}

type BadConfig struct {
    CheckerValidationError
}

func NewBadConfig(msg string) *BadConfig {
    return &BadConfig{
        CheckerValidationError: CheckerValidationError{
            CheckerException: CheckerException{message: msg},
        },
    }
}

type BadStructure struct {
    CheckerValidationError
}

func NewBadStructure(msg string) *BadStructure {
    return &BadStructure{
        CheckerValidationError: CheckerValidationError{
            CheckerException: CheckerException{message: msg},
        },
    }
}

type ExportError struct {
    CheckerException
}

func NewExportError(msg string) *ExportError {
    return &ExportError{
        CheckerException: CheckerException{message: msg},
    }
}

type TestingError struct {
    CheckerException
}

func NewTestingError(msg string) *TestingError {
    return &TestingError{
        CheckerException: CheckerException{message: msg},
    }
}

type PluginExecutionFailed struct {
    TestingError
    Output     string
    Percentage float64
}

func NewPluginExecutionFailed(message string, output string, percentage float64) *PluginExecutionFailed {
    return &PluginExecutionFailed{
        TestingError: TestingError{
            CheckerException: CheckerException{message: message},
        },
        Output:     output,
        Percentage: percentage,
    }
}

func IsBadConfig(err error) bool {
    _, ok := err.(*BadConfig)
    return ok
}

func IsBadStructure(err error) bool {
    _, ok := err.(*BadStructure)
    return ok
}

func IsExportError(err error) bool {
    _, ok := err.(*ExportError)
    return ok
}

func IsTestingError(err error) bool {
    _, ok := err.(*TestingError)
    return ok
}

func IsPluginExecutionFailed(err error) bool {
    _, ok := err.(*PluginExecutionFailed)
    return ok
}