import { test, expect } from "@playwright/test";
import path from "node:path";
import { readFileSync } from "node:fs";
const presets: Array<{
  id: string;
  name: string;
  manufacturer: number;
  product: number;
  serial: number;
  firmware: string;
  note: string;
}> = JSON.parse(
  readFileSync(new URL("../device-presets.json", import.meta.url), "utf8"),
);
const fixture = path.resolve("../testdata/web.input.bin");
async function upload(page: import("@playwright/test").Page) {
  await page.locator("#fit-file").setInputFiles({
    name: "Ultra Run.fit",
    mimeType: "application/octet-stream",
    buffer: await (await import("node:fs/promises")).readFile(fixture),
  });
}
test("inspect activity and source, select editable presets", async ({
  page,
}) => {
  await page.goto("/");
  await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
  await upload(page);
  await page
    .getByRole("button", { name: "Show basic activity details" })
    .click();
  await expect(page.locator("#details")).toContainText(
    "Garmin Fenix 9 Pro 47mm",
  );
  await expect(page.locator("#details")).toContainText("1.23 km");
  await page.getByRole("radio", { name: /Change recording device/ }).check();
  await page.getByRole("button", { name: "Inspect source identity" }).click();
  await expect(page.locator("#source-identity")).toContainText("6.38");
  for (const [preset, product, firmware] of [
    ["forerunner", "4315", "28.05"],
    ["edge", "3121", "9.75"],
    ["fenix", "4953", "6.38"],
  ]) {
    await page.getByLabel("Device preset").selectOption(preset);
    await expect(page.getByLabel("Product", { exact: true })).toHaveValue(
      product,
    );
    await expect(
      page.getByRole("spinbutton", { name: "Firmware (optional)" }),
    ).toHaveValue(firmware);
  }
  await page.getByRole("spinbutton", { name: "Firmware (optional)" }).fill("");
  await expect(
    page.getByRole("spinbutton", { name: "Firmware (optional)" }),
  ).toHaveValue("");
  await page.getByLabel("Device preset").selectOption("custom");
  for (const name of ["manufacturer", "product", "serial", "firmware"]) {
    await expect(page.locator(`input[name="${name}"]`)).toHaveValue("");
  }
  await page.screenshot({
    path: `test-results/workshop-${test.info().project.name}.png`,
    fullPage: true,
  });
});
for (const action of ["pauses", "continuous", "identity"])
  test(`download ${action}`, async ({ page }) => {
    await page.goto("/");
    await upload(page);
    await page.locator(`input[value="${action}"]`).check();
    if (action === "identity")
      await page.getByLabel("Device preset").selectOption("forerunner");
    const pending = page.waitForEvent("download");
    await page.getByRole("button", { name: "Transform and download" }).click();
    const download = await pending;
    expect(download.suggestedFilename()).toMatch(/^Ultra-Run-.*\.fit$/);
    expect(await download.failure()).toBeNull();
    const expected = action === "identity" ? "web-identity-firmware.output.bin" : `web-${action}.output.bin`;
    expect(readFileSync((await download.path())!)).toEqual(
      readFileSync(path.resolve("../testdata", expected)),
    );
    await expect(page.getByRole("status")).toContainText("Download started");
  });
test("validation, malformed FIT and network errors are visible", async ({
  page,
}) => {
  await page.goto("/");
  await page
    .getByRole("button", { name: "Show basic activity details" })
    .click();
  await expect(page.getByRole("alert")).toContainText("Choose a FIT file");
  await page.locator("#fit-file").setInputFiles({
    name: "bad.fit",
    mimeType: "application/octet-stream",
    buffer: Buffer.from("not fit"),
  });
  await page.getByRole("button", { name: "Transform and download" }).click();
  await expect(page.getByRole("alert")).toContainText("FIT");
  await upload(page);
  await page.route("**/activity-details", (route) => route.abort());
  await page
    .getByRole("button", { name: "Show basic activity details" })
    .click();
  await expect(page.getByRole("alert")).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Show basic activity details" }),
  ).toBeEnabled();
});

test.describe("runtime presets", () => {
  test("JSON presets populate editable options", async ({
    page,
  }) => {
    await page.goto("/");
    await expect(page.locator("#watch-preset option")).toHaveCount(
      presets.length + 1,
    );
    for (const preset of presets) {
      const option = page.locator(`#watch-preset option[value="${preset.id}"]`);
      await expect(option).toHaveText(preset.name);
      for (const field of [
        "manufacturer",
        "product",
        "serial",
        "firmware",
        "note",
      ] as const) {
        await expect(option).toHaveAttribute(
          `data-${field}`,
          String(preset[field]),
        );
      }
    }
  });
});

test("preset fetch failure leaves custom device entry available", async ({ page }) => {
  await page.route("**/device-presets", route => route.fulfill({ status: 500, json: { error: "cannot read configured device presets file" } }));
  await page.goto("/");
  await page.getByRole("radio", { name: /Change recording device/ }).check();
  await expect(page.locator("#preset-note")).toContainText("enter device details manually");
  await expect(page.getByLabel("Device preset")).toHaveValue("custom");
  await page.locator('input[name="serial"]').fill("100000009");
  await expect(page.locator('input[name="serial"]')).toHaveValue("100000009");
});

test("refresh fetches new presets and renders labels as text", async ({ page }) => {
  let name = '<img src=x onerror="alert(1)">';
  await page.route("**/device-presets", route => route.fulfill({ json: [{ ...presets[0], name }] }));
  await page.goto("/");
  await expect(page.locator('#watch-preset option[value="forerunner"]')).toHaveText(name);
  await expect(page.locator("#watch-preset img")).toHaveCount(0);
  name = "New device";
  await page.reload();
  await expect(page.locator('#watch-preset option[value="forerunner"]')).toHaveText(name);
});
