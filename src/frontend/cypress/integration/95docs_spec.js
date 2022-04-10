describe("Docs menu", function () {
    it("should open API docs", function () {
        cy.visit("/docs/api/");
        cy.get("body").contains("wakeci API documentation");
    });

    it("should open syntax docs", function () {
        cy.visit("/syntax/");
        cy.login();
        cy.get("body").contains("Ask a cow to say something smart");
    });
});
