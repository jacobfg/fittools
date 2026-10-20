# Third-party notices

`internal/fit/profile.json` contains manufacturer, Garmin product, sport and
sub-sport names derived from Garmin's official Python FIT SDK 21.214.0, with
presentation formatting and three product-name overrides from this project.
The table is frozen; see `testdata/README.md` for the reviewed update workflow.

Copyright 2026 Garmin International, Inc.
Licensed under the Flexible and Interoperable Data Transfer (FIT) Protocol
License; you may not use this data except in compliance with that license.

Source: https://github.com/garmin/fit-python-sdk
Protocol license: https://developer.garmin.com/fit/download/

Vite, TypeScript, Tailwind CSS, and Playwright are development/build dependencies.
Their license notices are distributed with their npm packages. The official `@garmin/fitsdk` JavaScript SDK 21.218.0 is a development-only
independent validator, distributed under Garmin’s FIT protocol license. Its
source and license are included in its npm package (https://github.com/garmin/fit-javascript-sdk).
Neither the Node toolchain nor any Garmin SDK is included in the runtime image.
