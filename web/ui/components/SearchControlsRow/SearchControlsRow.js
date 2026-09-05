/* ============================================================
   SearchControlsRow — the search page's filtering bar: a
   min-rating range slider, a custom sort dropdown and an
   "All SCIDs" toggle, with an optional status readout.
   Pure builder, no DOM side effects. Populates window.UI.
   The server page ships a static .controls-row in index.html;
   this builder is the canonical way to create new instances
   (and the migration target for the static row).
   Styling: SearchControlsRow.css (single source of truth).
   ============================================================ */
(function () {
  "use strict";
  const UI = (window.UI = window.UI || {});

  const DEFAULT_SORT_OPTIONS = [
    { value: "top_rated", label: "Top Rated" },
    { value: "name_asc", label: "Name A → Z" },
    { value: "name_desc", label: "Name Z → A" },
    { value: "newest", label: "Newest SCID" },
    { value: "oldest", label: "Oldest SCID" },
  ];

  function makeSelect(opts) {
    const el = document.createElement("div");
    el.className = "custom-select";
    if (opts.id) el.id = opts.id;

    const trigger = document.createElement("button");
    trigger.className = "custom-select-trigger";
    trigger.type = "button";
    const textEl = document.createElement("span");
    textEl.className = "custom-select-text";
    const arrow = document.createElement("span");
    arrow.className = "custom-select-arrow";
    arrow.textContent = "▼";
    trigger.append(textEl, arrow);

    const menu = document.createElement("ul");
    menu.className = "custom-select-menu";

    const getValue = () => {
      const sel = menu.querySelector("li.selected");
      return sel ? sel.getAttribute("data-value") : null;
    };

    function render(options) {
      menu.replaceChildren();
      options.forEach((opt) => {
        const li = document.createElement("li");
        li.setAttribute("data-value", opt.value);
        li.textContent = opt.label;
        if (opt.value === opts.value) li.classList.add("selected");
        li.addEventListener("click", () => {
          menu.querySelectorAll("li").forEach((l) => l.classList.remove("selected"));
          li.classList.add("selected");
          textEl.textContent = li.textContent;
          el.classList.remove("open");
          if (opts.onChange) opts.onChange(opt.value);
        });
        menu.appendChild(li);
      });
    }

    function syncText() {
      const sel = menu.querySelector("li.selected");
      textEl.textContent = sel ? sel.textContent : "";
    }

    trigger.addEventListener("click", (e) => {
      e.stopPropagation();
      el.classList.toggle("open");
    });
    document.addEventListener("click", () => el.classList.remove("open"));
    el.addEventListener("keydown", (e) => {
      if (e.key === "Escape") el.classList.remove("open");
    });

    render(opts.options || DEFAULT_SORT_OPTIONS);
    syncText();

    el.append(trigger, menu);
    return { element: el, trigger, textEl, menuEl: menu, getValue, setValue(opt) {
      menu.querySelectorAll("li").forEach((l) => {
        if (l.getAttribute("data-value") === opt) { l.classList.add("selected"); textEl.textContent = l.textContent; }
        else l.classList.remove("selected");
      });
    } };
  }

  /**
   * @param {Object=} opts
   * @param {string=} opts.minRatingId - id for the range input (default "minRating")
   * @param {string=} opts.minRatingValId - id for the value readout (default "minRatingVal")
   * @param {number=} opts.minRating - initial value 0-99 (default 30)
   * @param {function(number)=} opts.onMinRating - called with the new rating
   * @param {string=} opts.sortSelectId - id for the dropdown (default "sortMode")
   * @param {Array.<{value: string, label: string}>=} opts.sortOptions - sort modes
   * @param {string=} opts.sortValue - initially selected mode (default "top_rated")
   * @param {function(string)=} opts.onSort - called with the selected mode
   * @param {string=} opts.showAllId - id for the toggle checkbox (default "showAllToggle")
   * @param {string=} opts.showAllLabel - toggle label (default "All SCIDs")
   * @param {boolean=} opts.showAll - initial checked state
   * @param {function(boolean)=} opts.onShowAll - called with the checked state
   * @param {string|Node=} opts.status - status element or initial text for the readout
   * @param {string=} opts.statusId - id for the status readout (default "search-status")
   * @returns {{element: HTMLElement, minRatingEl: HTMLElement, minRatingValEl: HTMLElement, sortSelect: Object, showAllToggle: HTMLElement, statusEl: HTMLElement, setStatus: function(string): void}}
   */
  UI.SearchControlsRow = function SearchControlsRow(opts = {}) {
    const el = document.createElement("div");
    el.className = "controls-row";
    if (opts.id) el.id = opts.id;

    const minRating = opts.minRating == null ? 30 : opts.minRating;

    const ratingGroup = document.createElement("div");
    ratingGroup.className = "control-group";
    const ratingLabel = document.createElement("label");
    ratingLabel.htmlFor = opts.minRatingId || "minRating";
    ratingLabel.textContent = "Min rating:";
    const ratingVal = document.createElement("span");
    ratingVal.id = opts.minRatingValId || "minRatingVal";
    ratingVal.textContent = String(minRating);
    const ratingInput = document.createElement("input");
    ratingInput.type = "range";
    ratingInput.id = opts.minRatingId || "minRating";
    ratingInput.min = "0";
    ratingInput.max = "99";
    ratingInput.step = "1";
    ratingInput.value = String(minRating);
    ratingInput.addEventListener("input", () => {
      ratingVal.textContent = ratingInput.value;
      if (opts.onMinRating) opts.onMinRating(Number(ratingInput.value));
    });
    ratingGroup.append(ratingLabel, ratingVal, ratingInput);

    const sortGroup = document.createElement("div");
    sortGroup.className = "control-group";
    const sortLabel = document.createElement("label");
    sortLabel.textContent = "Sort:";
    const sortSelect = makeSelect({
      id: opts.sortSelectId || "sortMode",
      options: opts.sortOptions,
      value: opts.sortValue || "top_rated",
      onChange: opts.onSort,
    });
    sortGroup.append(sortLabel, sortSelect.element);

    const showAllGroup = document.createElement("div");
    showAllGroup.className = "control-group";
    const showAllText = document.createElement("label");
    showAllText.htmlFor = opts.showAllId || "showAllToggle";
    showAllText.textContent = opts.showAllLabel || "All SCIDs";
    const toggle = document.createElement("label");
    toggle.className = "toggle-switch";
    const toggleInput = document.createElement("input");
    toggleInput.type = "checkbox";
    toggleInput.id = opts.showAllId || "showAllToggle";
    if (opts.showAll) toggleInput.checked = true;
    toggleInput.addEventListener("change", () => {
      if (opts.onShowAll) opts.onShowAll(toggleInput.checked);
    });
    const toggleSlider = document.createElement("span");
    toggleSlider.className = "toggle-slider";
    toggle.append(toggleInput, toggleSlider);
    showAllGroup.append(showAllText, toggle);

    let statusEl = null;
    if (opts.status && opts.status.nodeType) statusEl = opts.status;
    else {
      statusEl = document.createElement("div");
      statusEl.id = opts.statusId || "search-status";
      statusEl.textContent = opts.status || "";
    }

    el.append(ratingGroup, sortGroup, showAllGroup, statusEl);

    return {
      element: el,
      minRatingEl: ratingInput,
      minRatingValEl: ratingVal,
      sortSelect,
      showAllToggle: toggleInput,
      statusEl,
      setStatus(text) { statusEl.textContent = text; },
    };
  };
})();