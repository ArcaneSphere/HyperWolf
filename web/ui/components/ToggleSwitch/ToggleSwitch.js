/* ============================================================
   ToggleSwitch — on/off checkbox styled as a sliding switch.
   Pure builder, no DOM side effects. Populates window.UI.
   App bindings (initToggleSwitch) attach change handlers to the
   returned label's input; the static markup in index.html matches
   this output 1:1.
   Styling: ToggleSwitch.css (mirrors style.css until visual baseline).
   ============================================================ */
(function () {
  "use strict";
  const UI = (window.UI = window.UI || {});

  /**
   * @param {boolean=} checked
   * @param {{id?: string, name?: string}=} attrs — optional id/name for the input
   * @returns {HTMLInputElement} label.toggle-switch > input + span.toggle-slider
   */
  UI.ToggleSwitch = function ToggleSwitch(checked = false, attrs = {}) {
    const label = document.createElement("label");
    label.className = "toggle-switch";

    const input = document.createElement("input");
    input.type = "checkbox";
    input.checked = !!checked;
    if (attrs.id) input.id = attrs.id;
    if (attrs.name) input.name = attrs.name;

    const slider = document.createElement("span");
    slider.className = "toggle-slider";

    label.append(input, slider);
    return label;
  };
})();