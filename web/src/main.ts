import "./style.css";
const el = <T extends HTMLElement>(id: string) =>
  document.getElementById(id) as T;
const form = el<HTMLFormElement>("tool-form");
const file = el<HTMLInputElement>("fit-file");
const status = el("status");
const error = el("error");
let busy = false;
let generation = 0;
let controller: AbortController | undefined;
function action() {
  return (
    form.querySelector('input[name="action"]:checked') as HTMLInputElement
  ).value;
}
function field(name: string) {
  return form.elements.namedItem(name) as HTMLInputElement;
}
function showError(message: string) {
  error.textContent = message;
  error.hidden = false;
  status.hidden = true;
}
function clearMessages() {
  error.hidden = true;
  status.hidden = true;
}
function updateAction() {
  const identity = action() === "identity";
  el("identity-fields").hidden = !identity;
  ["manufacturer", "product", "serial"].forEach(
    (n) => (field(n).required = identity),
  );
}
form
  .querySelectorAll('input[name="action"]')
  .forEach((input) => input.addEventListener("change", updateAction));
el<HTMLSelectElement>("watch-preset").addEventListener("change", (event) => {
  const preset = (event.target as HTMLSelectElement).selectedOptions[0];
  ["manufacturer", "product", "serial", "firmware"].forEach(
    (n) => (field(n).value = preset.dataset[n] ?? ""),
  );
  el("preset-note").textContent = preset.dataset.note ?? "";
});
function fileChanged() {
  generation++;
  controller?.abort();
  clearMessages();
  el("details").hidden = true;
  el("source-identity").textContent =
    "Source identity is detected automatically.";
  const f = file.files?.[0];
  el("file-title").textContent = f?.name ?? "Drop your FIT file here";
  el("file-meta").textContent = f
    ? `${(f.size / 1024 / 1024).toFixed(2)} MiB · ready to process`
    : "or click to browse · up to 100 MiB";
}
file.addEventListener("change", fileChanged);
const dropzone = el("dropzone");
["dragenter", "dragover"].forEach((name) =>
  dropzone.addEventListener(name, (e) => {
    e.preventDefault();
    dropzone.classList.add("dragging");
  }),
);
["dragleave", "drop"].forEach((name) =>
  dropzone.addEventListener(name, (e) => {
    e.preventDefault();
    dropzone.classList.remove("dragging");
  }),
);
dropzone.addEventListener("drop", (e) => {
  const files = (e as DragEvent).dataTransfer?.files;
  if (!files?.length) return;
  if (files.length !== 1) {
    showError("Choose a single FIT activity.");
    return;
  }
  file.files = files;
  fileChanged();
});
function chosenFile() {
  const f = file.files?.[0];
  if (!f) throw new Error("Choose a FIT file first.");
  if (!/\.fit$/i.test(f.name))
    throw new Error("The uploaded file must have a .fit extension.");
  if (f.size > 100 * 1024 * 1024)
    throw new Error("The FIT upload exceeds the 100 MiB limit.");
  return f;
}
function setBusy(value: boolean) {
  busy = value;
  form.setAttribute("aria-busy", String(value));
  form
    .querySelectorAll<HTMLButtonElement>("button")
    .forEach((b) => (b.disabled = value));
}
async function request(path: string, body: FormData) {
  const response = await fetch(path, {
    method: "POST",
    body,
    signal: controller?.signal,
  });
  if (!response.ok) {
    const result = await response.json().catch(() => ({
      error: `Request failed (${response.status}). Please try again.`,
    }));
    throw new Error(result.error);
  }
  return response;
}
async function operation(
  label: string,
  work: (body: FormData, version: number) => Promise<void>,
) {
  if (busy) return;
  clearMessages();
  try {
    const f = chosenFile();
    controller = new AbortController();
    setBusy(true);
    status.textContent = label;
    status.hidden = false;
    const body = new FormData();
    body.append("fit_file", f);
    await work(body, generation);
  } catch (e) {
    if (e instanceof DOMException && e.name === "AbortError") return;
    showError(
      e instanceof Error
        ? e.message
        : "Something went wrong. Please try again.",
    );
  } finally {
    setBusy(false);
  }
}
function duration(value: number | null) {
  if (value == null) return "Not reported";
  const s = Math.round(value);
  return `${Math.floor(s / 3600)}:${String(Math.floor((s % 3600) / 60)).padStart(2, "0")}:${String(s % 60).padStart(2, "0")}`;
}
function renderRows(rows: [string, unknown][]) {
  const dl = el("details-list");
  dl.replaceChildren();
  for (const [label, value] of rows) {
    const dt = document.createElement("dt");
    dt.textContent = label;
    const dd = document.createElement("dd");
    dd.textContent = value == null ? "Not reported" : String(value);
    dl.append(dt, dd);
  }
  el("details").hidden = false;
}
el("details-button").addEventListener("click", () =>
  operation("Reading activity details…", async (body, version) => {
    const result = await (await request("/activity-details", body)).json();
    if (version !== generation) return;
    const d = result.device,
      a = result.activity;
    const named = (v: { name: string | null; number: number }) =>
      v.name ? `${v.name} (${v.number})` : String(v.number);
    renderRows([
      ["Device", d.name],
      ["Serial", d.serial],
      ["Manufacturer", named(d.manufacturer)],
      ["Product", named(d.product)],
      ["Firmware", d.firmware],
      ["Activity type", a.sport],
      ["Activity subtype", a.sub_sport],
      [
        "Distance",
        a.distance_metres == null
          ? null
          : a.distance_metres >= 1000
            ? `${(a.distance_metres / 1000).toFixed(2)} km`
            : `${Math.round(a.distance_metres)} m`,
      ],
      ["Elapsed time", duration(a.elapsed_seconds)],
      ["Timer time", duration(a.timer_seconds)],
    ]);
    status.hidden = true;
  }),
);
el("inspect-button").addEventListener("click", () =>
  operation("Inspecting source identity…", async (body, version) => {
    const d = await (await request("/inspect-identity", body)).json();
    if (version !== generation) return;
    el("source-identity").textContent =
      `Detected source: manufacturer ${d.manufacturer}, product ${d.product}, FIT serial ${d.serial}, firmware ${d.firmware ?? "not reported"}.`;
    status.hidden = true;
  }),
);
form.addEventListener("submit", (event) => {
  event.preventDefault();
  void operation("Transforming your activity…", async (body, version) => {
    body.append("action", action());
    if (action() === "identity")
      for (const name of ["manufacturer", "product", "serial", "firmware"])
        body.append(name, field(name).value);
    const response = await request("/transform", body);
    const blob = await response.blob();
    if (version !== generation) return;
    const disposition = response.headers.get("Content-Disposition") ?? "";
    const filename =
      disposition.match(/filename="?([^";]+)"?/)?.[1] ?? "activity.fit";
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = url;
    link.download = filename;
    link.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
    status.textContent =
      "Your transformed FIT file is ready. Download started.";
  });
});
updateAction();

async function loadPresets() {
  try {
    const response = await fetch("/device-presets", { cache: "no-store" });
    const data = await response.json();
    if (!response.ok) throw new Error(data.error);
    const select = el<HTMLSelectElement>("watch-preset");
    for (const preset of data) {
      const option = document.createElement("option");
      option.value = preset.id;
      option.textContent = preset.name;
      for (const name of ["manufacturer", "product", "serial", "firmware", "note"])
        option.dataset[name] = String(preset[name]);
      select.append(option);
    }
  } catch (e) {
    el("preset-note").textContent = `Could not load device presets: ${e instanceof Error ? e.message : "request failed"}. You can enter device details manually.`;
  }
}
void loadPresets();
