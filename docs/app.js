const PAGE_SIZE = 50;

const state = {
  all: [],
  filtered: [],
  page: 0,
  sortKey: "served_date",
  sortDir: "desc",
};

const els = {
  q: document.getElementById("q"),
  proceeding: document.getElementById("proceeding"),
  docNumber: document.getElementById("docNumber"),
  from: document.getElementById("from"),
  to: document.getElementById("to"),
  hideUnavailable: document.getElementById("hideUnavailable"),
  clear: document.getElementById("clear"),
  rows: document.getElementById("rows"),
  summary: document.getElementById("summary"),
  totalCount: document.getElementById("totalCount"),
  prevPage: document.getElementById("prevPage"),
  nextPage: document.getElementById("nextPage"),
  pageInfo: document.getElementById("pageInfo"),
};

async function init() {
  const res = await fetch("data/documents.json");
  state.all = await res.json();
  els.totalCount.textContent = state.all.length.toLocaleString();

  for (const el of [els.q, els.proceeding, els.docNumber, els.from, els.to]) {
    el.addEventListener("input", () => {
      state.page = 0;
      applyFilters();
    });
  }
  els.hideUnavailable.addEventListener("change", () => {
    state.page = 0;
    applyFilters();
  });
  els.clear.addEventListener("click", clearFilters);
  els.prevPage.addEventListener("click", () => changePage(-1));
  els.nextPage.addEventListener("click", () => changePage(1));

  document.querySelectorAll("th.sortable").forEach((th) => {
    th.addEventListener("click", () => {
      const key = th.dataset.sort;
      if (state.sortKey === key) {
        state.sortDir = state.sortDir === "asc" ? "desc" : "asc";
      } else {
        state.sortKey = key;
        state.sortDir = "asc";
      }
      applyFilters();
    });
  });

  applyFilters();
}

function clearFilters() {
  els.q.value = "";
  els.proceeding.value = "";
  els.docNumber.value = "";
  els.from.value = "";
  els.to.value = "";
  els.hideUnavailable.checked = true;
  state.page = 0;
  applyFilters();
}

function applyFilters() {
  const q = els.q.value.trim().toLowerCase();
  const proceeding = els.proceeding.value.trim().toLowerCase();
  const docNumber = els.docNumber.value.trim();
  const from = els.from.value;
  const to = els.to.value;
  const hideUnavailable = els.hideUnavailable.checked;

  state.filtered = state.all.filter((d) => {
    if (hideUnavailable && d.unavailable) return false;
    if (q && !d.description.toLowerCase().includes(q) && !d.proceeding_title.toLowerCase().includes(q)) return false;
    if (proceeding && !d.proceeding_number.toLowerCase().includes(proceeding)) return false;
    if (docNumber && String(d.document_number) !== docNumber) return false;
    if (from && (!d.served_date || d.served_date < from)) return false;
    if (to && (!d.served_date || d.served_date > to)) return false;
    return true;
  });

  sortFiltered();
  render();
}

function sortFiltered() {
  const { sortKey, sortDir } = state;
  const dir = sortDir === "asc" ? 1 : -1;
  state.filtered.sort((a, b) => {
    let av = a[sortKey] ?? "";
    let bv = b[sortKey] ?? "";
    if (sortKey === "document_number") {
      av = a.document_number;
      bv = b.document_number;
      return (av - bv) * dir;
    }
    if (av < bv) return -1 * dir;
    if (av > bv) return 1 * dir;
    return 0;
  });
}

function render() {
  const total = state.filtered.length;
  const pageCount = Math.max(1, Math.ceil(total / PAGE_SIZE));
  state.page = Math.min(state.page, pageCount - 1);

  els.summary.textContent = `${total.toLocaleString()} matching document${total === 1 ? "" : "s"}`;

  const start = state.page * PAGE_SIZE;
  const pageRows = state.filtered.slice(start, start + PAGE_SIZE);

  els.rows.innerHTML = pageRows.map(rowHTML).join("");

  els.prevPage.disabled = state.page === 0;
  els.nextPage.disabled = state.page >= pageCount - 1;
  els.pageInfo.textContent = total === 0 ? "" : `Page ${state.page + 1} of ${pageCount}`;
}

function rowHTML(d) {
  const unavailable = d.unavailable
    ? `<span class="unavailable-badge">file unavailable (dead link on source site)</span>`
    : "";
  return `<tr>
    <td>${escapeHTML(d.served_date || "—")}</td>
    <td class="proceeding-number">${escapeHTML(d.proceeding_number)}</td>
    <td>${escapeHTML(d.proceeding_title)}</td>
    <td>${d.document_number}</td>
    <td>${escapeHTML(d.description)}${unavailable}</td>
  </tr>`;
}

function changePage(delta) {
  state.page += delta;
  render();
}

function escapeHTML(s) {
  return String(s).replace(/[&<>"']/g, (c) => ({
    "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;",
  }[c]));
}

init();
