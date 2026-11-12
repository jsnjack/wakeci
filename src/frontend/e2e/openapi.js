// Checks real API responses against the Swagger 2.0 spec wakeci serves at
// /docs/swagger.json

// resolve follows $ref and returns the schema with the definition's name
function resolve(spec, schema) {
    let name = "";
    while (schema && schema.$ref) {
        name = schema.$ref.replace("#/definitions/", "");
        schema = spec.definitions[name];
    }
    return { schema, name };
}

// validate reports in problems where value differs from schema, and records
// in seen every documented field it met, as "Definition.field"
export function validate(spec, schemaOrRef, value, where, problems, seen) {
    const { schema, name } = resolve(spec, schemaOrRef);
    if (!schema || Object.keys(schema).length === 0) {
        return;
    }
    const type = schema.type || (schema.properties ? "object" : "");
    if (value === null) {
        if (schema["x-nullable"] || schemaOrRef["x-nullable"]) {
            return;
        }
        problems.push(`${where} is null, documented as ${type || "a value"}`);
        return;
    }
    switch (type) {
        case "object": {
            if (typeof value !== "object" || Array.isArray(value)) {
                problems.push(`${where} is ${JSON.stringify(value)?.slice(0, 40)}, documented as an object`);
                return;
            }
            const properties = schema.properties || {};
            for (const [key, field] of Object.entries(value)) {
                if (key in properties) {
                    if (name) {
                        seen.add(`${name}.${key}`);
                    }
                    validate(spec, properties[key], field, `${where}.${key}`, problems, seen);
                } else if (schema.additionalProperties) {
                    validate(spec, schema.additionalProperties, field, `${where}.${key}`, problems, seen);
                } else {
                    problems.push(`${where}.${key} is returned but not documented${name ? ` in ${name}` : ""}`);
                }
            }
            return;
        }
        case "array":
            if (!Array.isArray(value)) {
                problems.push(`${where} is ${typeof value}, documented as an array`);
                return;
            }
            value.forEach((item, i) => validate(spec, schema.items, item, `${where}[${i}]`, problems, seen));
            return;
        case "integer":
            if (!Number.isInteger(value)) {
                problems.push(`${where} is ${JSON.stringify(value)}, documented as an integer`);
            }
            return;
        case "number":
        case "string":
        case "boolean":
            if (typeof value !== type) {
                problems.push(`${where} is ${typeof value}, documented as ${type}`);
            }
    }
}

// documentedFields lists "Definition.field" for every field of the
// definitions that responses use
export function documentedFields(spec) {
    const fields = [];
    for (const [name, definition] of Object.entries(spec.definitions)) {
        for (const field of Object.keys(definition.properties || {})) {
            fields.push(`${name}.${field}`);
        }
    }
    return fields;
}
