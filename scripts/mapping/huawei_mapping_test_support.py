#!/usr/bin/env python3
"""Dependency-free test helpers for the Huawei mapping gate self-tests.

The self-tests run as plain scripts from scripts/check_docs.sh and are also
collected by pytest. `parametrize` runs every case inside one test function and
names the failing case, `Raises` has the semantics of the pytest context manager
that the tests use, and `run` executes every `test_*` function of a module.
`fraction_of` and `kernel_decimal` convert between a kernel Decimal and an exact
fraction, so that a test computes an SI value independently of the validator.
"""
from __future__ import annotations

import os
import re
import sys
import traceback
from fractions import Fraction


def fraction_of(number):
    """The exact value of a kernel Decimal {coefficient, exponent10}."""
    return Fraction(int(number["coefficient"])) * Fraction(10) ** number["exponent10"]


def kernel_decimal(value):
    """The canonical kernel Decimal of an exact fraction; a value that does not terminate in decimal fails."""
    value, exponent = Fraction(value), 0
    while value.denominator != 1:
        value *= 10
        exponent -= 1
        if exponent < -18:
            raise AssertionError("value does not terminate in decimal")
    coefficient = value.numerator
    if coefficient == 0:
        return {"coefficient": "0", "exponent10": 0}
    while coefficient % 10 == 0:
        coefficient //= 10
        exponent += 1
    return {"coefficient": str(coefficient), "exponent10": exponent}


class Raises:
    def __init__(self, exception, match=None):
        self.exception = exception
        self.match = match
        self.value = None

    def __enter__(self):
        return self

    def __exit__(self, kind, value, trace):
        if kind is None:
            raise AssertionError("DID NOT RAISE %s" % self.exception.__name__)
        if not issubclass(kind, self.exception):
            return False
        if self.match is not None and not re.search(self.match, str(value)):
            raise AssertionError("%r does not match %r" % (str(value), self.match))
        self.value = value
        return True


def parametrize(names, cases):
    parameters = [name.strip() for name in names.split(",")] if isinstance(names, str) else list(names)

    def decorate(function):
        def wrapper():
            for index, case in enumerate(cases):
                arguments = tuple(case) if len(parameters) > 1 else (case,)
                try:
                    function(*arguments)
                except Exception as error:
                    raise AssertionError("%s case %d (%s): %s: %s" % (function.__name__, index, ", ".join(repr(item)[:60] for item in arguments), type(error).__name__, error)) from error

        wrapper.__name__ = function.__name__
        wrapper.__doc__ = function.__doc__
        wrapper.case_count = len(cases)
        return wrapper

    return decorate


def run(namespace):
    """Run every test_* function of a module namespace; return a process exit status."""
    tests = [(name, item) for name, item in namespace.items() if name.startswith("test_") and callable(item)]
    failures, cases = [], 0
    for name, function in tests:
        cases += getattr(function, "case_count", 1)
        try:
            function()
        except Exception:
            failures.append(name)
            print("FAIL", name, file=sys.stderr)
            traceback.print_exc()
    print("%s: %s; %d tests, %d cases" % (os.path.basename(namespace.get("__file__", "tests")), "FAIL" if failures else "PASS", len(tests), cases))
    return 1 if failures else 0
