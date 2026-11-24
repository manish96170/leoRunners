describe("built application", () => {
  it("renders the page", () => {
    cy.visit("/");
    cy.get("#app").should(
      "contain.text",
      "Hello, LeoRunners validation workload",
    );
  });

  it("serves the health endpoint", () => {
    cy.request("/health.json").its("body.status").should("eq", "ok");
  });

  it("ships the hidden build manifest", () => {
    cy.request("/.vite/manifest.json").its("status").should("eq", 200);
  });
});
