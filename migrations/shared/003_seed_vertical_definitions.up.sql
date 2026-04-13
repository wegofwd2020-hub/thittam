-- 003_seed_vertical_definitions.up.sql
-- Seeds the four built-in verticals from their YAML definitions.
-- The `config` column stores the full vertical config as JSONB (everything
-- under the `vertical:` key except id/name/version/description).

INSERT INTO vertical_definitions (id, name, version, description, config) VALUES

-- ──────────────────────────────────────────────────────────────────────
-- 1. Movie Production
-- ──────────────────────────────────────────────────────────────────────
(
  'movie-production',
  'Movie & Film Production',
  '1.0.0',
  'For film production companies and independent producers.',
  '{
    "entity_labels": {
      "project": "Production",
      "project_plural": "Productions",
      "phase": "Phase",
      "phase_plural": "Phases",
      "team_member": "Crew Member",
      "team_member_plural": "Crew Members",
      "rate_label": "Day Rate",
      "rate_unit": "day"
    },
    "phase_types": [
      {"id":"development","label":"Development","order":1,"is_billable":false,"allowed_transitions":["pre_production"]},
      {"id":"pre_production","label":"Pre-Production","order":2,"is_billable":false,"allowed_transitions":["production"]},
      {"id":"production","label":"Production","order":3,"is_billable":true,"allowed_transitions":["post_production"]},
      {"id":"post_production","label":"Post-Production","order":4,"is_billable":true,"allowed_transitions":["released"]},
      {"id":"released","label":"Released","order":5,"is_billable":false,"allowed_transitions":[]}
    ],
    "budget_categories": [
      {"id":"above_the_line","label":"Above the Line","description":"Key creative talent","default_account_code":"5100"},
      {"id":"below_the_line","label":"Below the Line","description":"Technical crew and production costs","default_account_code":"5200"},
      {"id":"post_production","label":"Post-Production","description":"Editing, VFX, sound","default_account_code":"5300"}
    ],
    "budget_templates": [
      {
        "name":"Feature Film",
        "description":"Standard feature film budget template",
        "line_items":[
          {"category_id":"above_the_line","description":"Director","account_code":"5100","default_amount":"500000","is_required":true},
          {"category_id":"below_the_line","description":"Cinematographer","account_code":"5200","default_amount":"200000","is_required":true}
        ]
      }
    ],
    "expense_categories": [
      {"id":"location_rental","label":"Location Rental","tax_treatment":"input_gst","default_account_code":"5200","requires_po":true},
      {"id":"equipment_rental","label":"Equipment Rental","tax_treatment":"input_gst","default_account_code":"5200","requires_po":true}
    ],
    "approval_workflow": {
      "limits":[
        {"role":"coordinator","max_amount":"200000"},
        {"role":"manager","max_amount":"1000000"}
      ],
      "dual_approval_above":"1000000"
    },
    "inventory_categories": [
      {"id":"camera","label":"Camera Equipment","is_trackable":true,"depreciation_years":5},
      {"id":"props","label":"Props","is_trackable":false,"depreciation_years":null}
    ],
    "report_definitions": [
      {"id":"cost_report","name":"Daily Cost Report","description":"Budget vs actuals by category","data_sources":["budget-planning","expense-tracking"],"default_columns":["category","budget","actual","variance"]}
    ],
    "custom_fields": {
      "project":[
        {"key":"call_sheet_time","label":"Call Sheet Time","type":"text","required":false}
      ],
      "expense":[
        {"key":"vendor_gstin","label":"Vendor GSTIN","type":"text","required":true}
      ]
    }
  }'::jsonb
),

-- ──────────────────────────────────────────────────────────────────────
-- 2. Software Development
-- ──────────────────────────────────────────────────────────────────────
(
  'software-development',
  'Software Development Services',
  '1.0.0',
  'For software development agencies, IT consultancies, product studios, and managed services providers. Supports fixed-price, T&M, and retainer engagements. Includes cloud infrastructure cost tracking and software license management.',
  '{
    "entity_labels": {
      "project": "Project",
      "project_plural": "Projects",
      "phase": "Milestone",
      "phase_plural": "Milestones",
      "team_member": "Team Member",
      "team_member_plural": "Team Members",
      "rate_label": "Hourly Rate",
      "rate_unit": "hour"
    },
    "phase_types": [
      {"id":"discovery","label":"Discovery & Scoping","order":1,"is_billable":true,"allowed_transitions":["design","proposal","cancelled"]},
      {"id":"proposal","label":"Proposal / Estimation","order":2,"is_billable":false,"allowed_transitions":["discovery","design","cancelled"]},
      {"id":"design","label":"UX Design & Architecture","order":3,"is_billable":true,"allowed_transitions":["development","discovery"]},
      {"id":"development","label":"Development (Sprint Cycle)","order":4,"is_billable":true,"allowed_transitions":["qa","design","on_hold"]},
      {"id":"qa","label":"Quality Assurance & Testing","order":5,"is_billable":true,"allowed_transitions":["uat","development"]},
      {"id":"uat","label":"User Acceptance Testing","order":6,"is_billable":true,"allowed_transitions":["go_live","development","qa"]},
      {"id":"go_live","label":"Go-Live & Deployment","order":7,"is_billable":true,"allowed_transitions":["support","completed"]},
      {"id":"support","label":"Post-Launch Support (Warranty / AMC)","order":8,"is_billable":true,"allowed_transitions":["completed"]},
      {"id":"on_hold","label":"On Hold","order":9,"is_billable":false,"allowed_transitions":["development","cancelled"]},
      {"id":"completed","label":"Completed","order":10,"is_billable":false,"allowed_transitions":["archived"]},
      {"id":"cancelled","label":"Cancelled","order":11,"is_billable":false,"allowed_transitions":["archived"]},
      {"id":"archived","label":"Archived","order":12,"is_billable":false,"allowed_transitions":[]}
    ],
    "budget_categories": [
      {"id":"personnel","label":"Personnel / Team Cost","description":"Salaries, contractor fees, and hourly billing for all team members","default_account_code":"5000"},
      {"id":"cloud_infrastructure","label":"Cloud Infrastructure","description":"AWS, GCP, Azure, or other cloud provider costs allocated to this project","default_account_code":"6000"},
      {"id":"software_licenses","label":"Software Licenses & Tools","description":"IDEs, project management tools, design tools, security scanners, CI/CD","default_account_code":"6100"},
      {"id":"third_party_services","label":"Third-Party APIs & Services","description":"Payment gateways, communication APIs, analytics, monitoring SaaS","default_account_code":"6200"},
      {"id":"qa_testing","label":"QA & Testing","description":"Test environment costs, automated testing tools, security audits, pen tests","default_account_code":"6300"},
      {"id":"devops_infrastructure","label":"DevOps & Infrastructure Setup","description":"CI/CD pipeline setup, Kubernetes cluster, monitoring stack","default_account_code":"6400"},
      {"id":"travel_onsite","label":"Travel & On-Site Expenses","description":"Client site visits, travel, accommodation, per diem","default_account_code":"6500"},
      {"id":"subcontractors","label":"Subcontractors & Freelancers","description":"External specialists engaged for specific deliverables","default_account_code":"6600"},
      {"id":"contingency","label":"Contingency","description":"Reserve for scope changes and risk — typically 10–15% of project cost","default_account_code":"8000"}
    ],
    "budget_templates": [
      {
        "name":"Fixed-Price Web Application",
        "description":"Typical budget for a fixed-scope web application project",
        "line_items":[
          {"category_id":"personnel","description":"Project Manager","account_code":"5001","is_required":true},
          {"category_id":"personnel","description":"UX / UI Designer","account_code":"5002","is_required":false},
          {"category_id":"personnel","description":"Backend Engineers","account_code":"5003","is_required":true},
          {"category_id":"personnel","description":"Frontend Engineers","account_code":"5004","is_required":true},
          {"category_id":"personnel","description":"QA Engineers","account_code":"5005","is_required":true},
          {"category_id":"personnel","description":"DevOps / SRE","account_code":"5006","is_required":false},
          {"category_id":"cloud_infrastructure","description":"Development Environment (AWS)","account_code":"6001","is_required":true},
          {"category_id":"cloud_infrastructure","description":"Staging Environment","account_code":"6002","is_required":true},
          {"category_id":"software_licenses","description":"Design & Collaboration Tools (Figma, Jira, Confluence)","account_code":"6101","is_required":false},
          {"category_id":"software_licenses","description":"CI/CD & Code Quality Tools","account_code":"6102","is_required":false},
          {"category_id":"third_party_services","description":"Payment Gateway Integration","account_code":"6201","is_required":false},
          {"category_id":"qa_testing","description":"Automated Testing Tools & Licences","account_code":"6301","is_required":false},
          {"category_id":"qa_testing","description":"Security Audit","account_code":"6302","is_required":false},
          {"category_id":"contingency","description":"Contingency Reserve (10%)","account_code":"8001","is_required":true}
        ]
      },
      {
        "name":"Time & Materials Retainer",
        "description":"Monthly retainer budget for a T&M engagement",
        "line_items":[
          {"category_id":"personnel","description":"Dedicated Engineering Team (monthly hours × rate)","account_code":"5003","is_required":true},
          {"category_id":"cloud_infrastructure","description":"Monthly Cloud Costs (passed through at cost)","account_code":"6001","is_required":true},
          {"category_id":"software_licenses","description":"Monthly Tool Subscriptions","account_code":"6101","is_required":false},
          {"category_id":"contingency","description":"Buffer (5%)","account_code":"8001","is_required":false}
        ]
      },
      {
        "name":"Mobile App (iOS + Android)",
        "description":"Cross-platform mobile application budget",
        "line_items":[
          {"category_id":"personnel","description":"Mobile Engineers (React Native / Flutter)","account_code":"5003","is_required":true},
          {"category_id":"personnel","description":"UX Designer","account_code":"5002","is_required":true},
          {"category_id":"personnel","description":"QA (Mobile Devices)","account_code":"5005","is_required":true},
          {"category_id":"software_licenses","description":"Apple Developer Program ($99/yr)","account_code":"6101","is_required":true},
          {"category_id":"software_licenses","description":"Google Play Console ($25 one-time)","account_code":"6101","is_required":true},
          {"category_id":"cloud_infrastructure","description":"Backend API Infrastructure","account_code":"6001","is_required":true},
          {"category_id":"third_party_services","description":"Push Notifications (FCM / APNs)","account_code":"6201","is_required":false},
          {"category_id":"contingency","description":"Contingency (12%)","account_code":"8001","is_required":true}
        ]
      }
    ],
    "expense_categories": [
      {"id":"salary","label":"Salary / Payroll","tax_treatment":"tds_applicable","default_account_code":"5000","requires_po":false},
      {"id":"contractor_invoice","label":"Contractor / Freelancer Invoice","tax_treatment":"tds_applicable","default_account_code":"6600","requires_po":true},
      {"id":"cloud_bill","label":"Cloud Infrastructure Bill","tax_treatment":"input_gst","default_account_code":"6001","requires_po":false},
      {"id":"saas_subscription","label":"SaaS / Software Subscription","tax_treatment":"input_gst","default_account_code":"6101","requires_po":false},
      {"id":"api_usage","label":"Third-Party API Usage","tax_treatment":"input_gst","default_account_code":"6201","requires_po":false},
      {"id":"hardware_purchase","label":"Hardware Purchase","tax_treatment":"input_gst","default_account_code":"6700","requires_po":true},
      {"id":"travel","label":"Travel & Accommodation","tax_treatment":"input_gst","default_account_code":"6501","requires_po":false},
      {"id":"training_certification","label":"Training & Certifications","tax_treatment":"input_gst","default_account_code":"6800","requires_po":false},
      {"id":"reimbursement","label":"Employee Reimbursement","tax_treatment":"none","default_account_code":"6099","requires_po":false}
    ],
    "approval_workflow": {
      "limits":[
        {"role":"accountant","max_amount":"50000"},
        {"role":"coordinator","max_amount":"500000"},
        {"role":"manager","max_amount":"2000000"}
      ],
      "dual_approval_above":"2000000"
    },
    "inventory_categories": [
      {"id":"laptop","label":"Laptop / Workstation","is_trackable":true,"depreciation_years":3},
      {"id":"monitor","label":"Monitor & Peripherals","is_trackable":true,"depreciation_years":5},
      {"id":"server","label":"On-Premise Server","is_trackable":true,"depreciation_years":5},
      {"id":"software_license","label":"Perpetual Software License","is_trackable":false,"depreciation_years":3},
      {"id":"mobile_device","label":"Mobile Device (Testing)","is_trackable":true,"depreciation_years":3},
      {"id":"networking","label":"Networking Equipment","is_trackable":true,"depreciation_years":5}
    ],
    "report_definitions": [
      {"id":"burn_rate","name":"Project Burn Rate","description":"Actual spend per week/sprint vs. planned budget consumption rate","data_sources":["budget-planning","expense-tracking"],"default_columns":["week","planned_burn","actual_burn","cumulative_actual","budget_remaining","projected_completion_cost"]},
      {"id":"project_pl","name":"Project Profitability (P&L)","description":"Revenue vs. cost per project to determine gross margin","data_sources":["general-ledger","budget-planning"],"default_columns":["project","revenue","cost","gross_margin","gross_margin_pct"]},
      {"id":"resource_utilisation","name":"Resource Utilisation Report","description":"Billable vs. non-billable hours per team member across projects","data_sources":["project-management","expense-tracking"],"default_columns":["team_member","role","billable_hours","non_billable_hours","utilisation_pct","cost"]},
      {"id":"invoice_tracker","name":"Client Invoice Tracker","description":"Invoices raised, paid, and outstanding per project milestone","data_sources":["general-ledger"],"default_columns":["project","milestone","invoice_no","amount","raised_date","due_date","paid_date","status"]},
      {"id":"cloud_cost_breakdown","name":"Cloud Cost Breakdown","description":"AWS / GCP / Azure costs allocated per project with trend","data_sources":["expense-tracking"],"default_columns":["project","provider","service","month","amount","vs_budget"]},
      {"id":"gstr2a","name":"GSTR-2A Purchase Register","description":"All vendor GST invoices for monthly input tax credit reconciliation","data_sources":["expense-tracking"],"default_columns":["vendor_gstin","invoice_no","invoice_date","taxable_value","cgst","sgst","igst","total"]}
    ],
    "custom_fields": {
      "project":[
        {"key":"client_name","label":"Client Name","type":"text","required":true},
        {"key":"engagement_type","label":"Engagement Type","type":"select","options":["Fixed-Price","Time & Materials","Retainer","Dedicated Team"],"required":true},
        {"key":"technology_stack","label":"Primary Technology Stack","type":"select","options":["React + Node.js","React + Go","Flutter","iOS Native","Android Native","Python / Django","Java Spring",".NET","Other"],"required":false},
        {"key":"client_po_number","label":"Client PO Number","type":"text","required":false},
        {"key":"sow_reference","label":"Statement of Work Reference","type":"text","required":false}
      ],
      "expense":[
        {"key":"sprint_number","label":"Sprint / Iteration Number","type":"text","required":false},
        {"key":"billable_to_client","label":"Billable to Client?","type":"select","options":["Yes","No","Partially"],"required":true},
        {"key":"jira_ticket","label":"Jira / Linear Ticket Reference","type":"text","required":false}
      ]
    }
  }'::jsonb
),

-- ──────────────────────────────────────────────────────────────────────
-- 3. Construction & Civil Engineering
-- ──────────────────────────────────────────────────────────────────────
(
  'construction',
  'Construction & Civil Engineering',
  '1.0.0',
  'For construction companies, civil engineering firms, EPC contractors, and infrastructure developers. Supports BOQ-based contracts, milestone billing, subcontractor management, material procurement, plant and equipment tracking, and retention accounting.',
  '{
    "entity_labels": {
      "project": "Contract",
      "project_plural": "Contracts",
      "phase": "Work Package",
      "phase_plural": "Work Packages",
      "team_member": "Site Worker",
      "team_member_plural": "Site Workers",
      "rate_label": "Day Rate",
      "rate_unit": "day"
    },
    "phase_types": [
      {"id":"tender","label":"Tender & Estimation","order":1,"is_billable":false,"allowed_transitions":["pre_construction","cancelled"]},
      {"id":"pre_construction","label":"Pre-Construction & Mobilisation","order":2,"is_billable":true,"allowed_transitions":["foundation","on_hold"]},
      {"id":"foundation","label":"Foundation & Earthworks","order":3,"is_billable":true,"allowed_transitions":["structure","pre_construction"]},
      {"id":"structure","label":"Structural Works","order":4,"is_billable":true,"allowed_transitions":["mep","finishing","on_hold"]},
      {"id":"mep","label":"MEP (Mechanical, Electrical & Plumbing)","order":5,"is_billable":true,"allowed_transitions":["finishing","structure"]},
      {"id":"finishing","label":"Finishing & Fit-Out","order":6,"is_billable":true,"allowed_transitions":["handover","mep"]},
      {"id":"handover","label":"Handover & Commissioning","order":7,"is_billable":true,"allowed_transitions":["defects_liability","completed"]},
      {"id":"defects_liability","label":"Defects Liability Period","order":8,"is_billable":false,"allowed_transitions":["completed"]},
      {"id":"on_hold","label":"On Hold","order":9,"is_billable":false,"allowed_transitions":["pre_construction","foundation","structure","cancelled"]},
      {"id":"completed","label":"Completed","order":10,"is_billable":false,"allowed_transitions":["archived"]},
      {"id":"cancelled","label":"Cancelled","order":11,"is_billable":false,"allowed_transitions":["archived"]},
      {"id":"archived","label":"Archived","order":12,"is_billable":false,"allowed_transitions":[]}
    ],
    "budget_categories": [
      {"id":"labour","label":"Labour","description":"Daily/weekly wages for site workers, skilled and unskilled labour","default_account_code":"5000"},
      {"id":"materials","label":"Materials & Procurement","description":"Cement, steel, bricks, sand, aggregates, finishes, fittings","default_account_code":"6000"},
      {"id":"plant_equipment","label":"Plant & Equipment","description":"Hired or owned machinery — cranes, excavators, mixers, scaffolding","default_account_code":"6100"},
      {"id":"subcontractors","label":"Subcontractors","description":"Specialist subcontractors for civil, structural, MEP, painting, etc.","default_account_code":"6200"},
      {"id":"site_overheads","label":"Site Overheads","description":"Site office rent, security, utilities, site manager, temporary works","default_account_code":"6300"},
      {"id":"professional_fees","label":"Professional Fees","description":"Architects, structural engineers, surveyors, quality inspectors","default_account_code":"6400"},
      {"id":"testing_inspection","label":"Testing & Inspection","description":"Soil testing, concrete cube tests, structural load tests, third-party QA","default_account_code":"6500"},
      {"id":"contingency","label":"Contingency","description":"Reserve for unforeseen site conditions and scope changes (typically 5–10%)","default_account_code":"8000"}
    ],
    "budget_templates": [
      {
        "name":"Residential Building (Per Floor)",
        "description":"Standard budget for a multi-storey residential construction project",
        "line_items":[
          {"category_id":"labour","description":"Structural Labour (Foundation & Slabs)","account_code":"5001","is_required":true},
          {"category_id":"labour","description":"Masonry & Plastering Labour","account_code":"5002","is_required":true},
          {"category_id":"labour","description":"Finishing Labour (Tiles, Paint, Carpentry)","account_code":"5003","is_required":true},
          {"category_id":"materials","description":"Cement","account_code":"6001","is_required":true},
          {"category_id":"materials","description":"Steel / TMT Bars","account_code":"6002","is_required":true},
          {"category_id":"materials","description":"Bricks / Blocks","account_code":"6003","is_required":true},
          {"category_id":"materials","description":"Sand & Aggregates","account_code":"6004","is_required":true},
          {"category_id":"materials","description":"Tiles & Flooring","account_code":"6005","is_required":false},
          {"category_id":"materials","description":"Doors, Windows & Fixtures","account_code":"6006","is_required":false},
          {"category_id":"plant_equipment","description":"Tower Crane Hire","account_code":"6101","is_required":false},
          {"category_id":"plant_equipment","description":"Concrete Mixer & Pump","account_code":"6102","is_required":true},
          {"category_id":"plant_equipment","description":"Scaffolding","account_code":"6103","is_required":true},
          {"category_id":"subcontractors","description":"Electrical Works (MEP Subcontractor)","account_code":"6201","is_required":true},
          {"category_id":"subcontractors","description":"Plumbing & Sanitary Works","account_code":"6202","is_required":true},
          {"category_id":"subcontractors","description":"Painting & Waterproofing","account_code":"6203","is_required":true},
          {"category_id":"site_overheads","description":"Site Office & Temporary Structures","account_code":"6301","is_required":true},
          {"category_id":"professional_fees","description":"Architect & Structural Engineer Fees","account_code":"6401","is_required":true},
          {"category_id":"testing_inspection","description":"Material Testing & Third-Party QA","account_code":"6501","is_required":false},
          {"category_id":"contingency","description":"Contingency Reserve (7%)","account_code":"8001","is_required":true}
        ]
      },
      {
        "name":"Civil Infrastructure (Roads / Drainage)",
        "description":"Budget for a civil works contract (road, drainage, or utility)",
        "line_items":[
          {"category_id":"labour","description":"Earthworks & Excavation Labour","account_code":"5001","is_required":true},
          {"category_id":"materials","description":"Aggregates & Sub-base Material","account_code":"6004","is_required":true},
          {"category_id":"materials","description":"Bitumen / Asphalt","account_code":"6007","is_required":false},
          {"category_id":"materials","description":"Pipes, Manholes & Drainage Fittings","account_code":"6008","is_required":false},
          {"category_id":"plant_equipment","description":"Excavator & Compactor Hire","account_code":"6101","is_required":true},
          {"category_id":"plant_equipment","description":"Road Roller & Paving Machine","account_code":"6104","is_required":false},
          {"category_id":"subcontractors","description":"Specialised Civil Subcontractor","account_code":"6204","is_required":false},
          {"category_id":"site_overheads","description":"Traffic Management & Safety","account_code":"6302","is_required":true},
          {"category_id":"testing_inspection","description":"Soil & Compaction Testing","account_code":"6502","is_required":true},
          {"category_id":"contingency","description":"Contingency (10%)","account_code":"8001","is_required":true}
        ]
      }
    ],
    "expense_categories": [
      {"id":"labour_wages","label":"Labour Wages","tax_treatment":"tds_applicable","default_account_code":"5000","requires_po":false},
      {"id":"material_purchase","label":"Material Purchase","tax_treatment":"input_gst","default_account_code":"6000","requires_po":true},
      {"id":"plant_hire","label":"Plant & Equipment Hire","tax_treatment":"input_gst","default_account_code":"6101","requires_po":true},
      {"id":"subcontractor_bill","label":"Subcontractor Bill","tax_treatment":"tds_applicable","default_account_code":"6200","requires_po":true},
      {"id":"site_overhead","label":"Site Overhead (Utilities, Security)","tax_treatment":"input_gst","default_account_code":"6300","requires_po":false},
      {"id":"professional_fee","label":"Professional Fee (Architect / Engineer)","tax_treatment":"tds_applicable","default_account_code":"6400","requires_po":true},
      {"id":"petty_cash","label":"Petty Cash (Site)","tax_treatment":"none","default_account_code":"6099","requires_po":false}
    ],
    "approval_workflow": {
      "limits":[
        {"role":"accountant","max_amount":"50000"},
        {"role":"coordinator","max_amount":"500000"},
        {"role":"manager","max_amount":"5000000"}
      ],
      "dual_approval_above":"5000000"
    },
    "inventory_categories": [
      {"id":"heavy_machinery","label":"Heavy Machinery (Owned)","is_trackable":true,"depreciation_years":10},
      {"id":"small_plant","label":"Small Plant & Tools","is_trackable":true,"depreciation_years":5},
      {"id":"scaffolding","label":"Scaffolding & Formwork","is_trackable":true,"depreciation_years":7},
      {"id":"vehicle","label":"Site Vehicles","is_trackable":true,"depreciation_years":5},
      {"id":"safety_equipment","label":"Safety Equipment (PPE, Barriers)","is_trackable":false,"depreciation_years":null},
      {"id":"temporary_structures","label":"Temporary Site Structures","is_trackable":false,"depreciation_years":null}
    ],
    "report_definitions": [
      {"id":"boq_variance","name":"BOQ Variance Report","description":"Bill of Quantities budgeted vs. actual quantities and costs per work item","data_sources":["budget-planning","expense-tracking"],"default_columns":["item_code","description","unit","boq_qty","boq_rate","boq_amount","actual_qty","actual_rate","actual_amount","variance"]},
      {"id":"progress_bill","name":"Progress Billing Statement","description":"Milestone-based billing statement for client (RA Bill format)","data_sources":["budget-planning","expense-tracking","general-ledger"],"default_columns":["milestone","description","contract_value","completed_pct","amount_due","retention_held","net_payable"]},
      {"id":"retention_schedule","name":"Retention Money Schedule","description":"Retention held from subcontractors and due from client","data_sources":["general-ledger"],"default_columns":["party","contract_value","retention_rate","retention_held","retention_released","balance_held","release_date"]},
      {"id":"plant_utilisation","name":"Plant & Equipment Utilisation","description":"Hired and owned plant utilisation rates and costs per project","data_sources":["inventory-management","expense-tracking"],"default_columns":["asset_code","description","days_deployed","idle_days","utilisation_pct","hire_cost","depreciation"]},
      {"id":"subcontractor_ledger","name":"Subcontractor Ledger","description":"Bills received, retention deducted, TDS, and payments made per subcontractor","data_sources":["expense-tracking","general-ledger"],"default_columns":["subcontractor","total_billed","retention_deducted","tds_deducted","net_paid","balance_due"]},
      {"id":"material_reconciliation","name":"Material Reconciliation","description":"Materials received vs. issued vs. on-site stock balance","data_sources":["inventory-management","expense-tracking"],"default_columns":["material","unit","opening_stock","received","issued","closing_stock","wastage_pct"]}
    ],
    "custom_fields": {
      "project":[
        {"key":"client_name","label":"Client / Owner Name","type":"text","required":true},
        {"key":"contract_type","label":"Contract Type","type":"select","options":["Lump Sum","Bill of Quantities","Cost Plus","EPC / Turnkey","PPP"],"required":true},
        {"key":"contract_number","label":"Contract / Work Order Number","type":"text","required":true},
        {"key":"site_location","label":"Site Location / Address","type":"text","required":true},
        {"key":"rera_registration","label":"RERA Registration Number","type":"text","required":false},
        {"key":"completion_certificate_no","label":"Completion Certificate Number","type":"text","required":false}
      ],
      "expense":[
        {"key":"work_package","label":"Work Package / BOQ Item","type":"text","required":false},
        {"key":"ra_bill_number","label":"RA Bill / Progress Bill Number","type":"text","required":false},
        {"key":"material_received_note","label":"Material Received Note (MRN) Number","type":"text","required":false}
      ]
    }
  }'::jsonb
),

-- ──────────────────────────────────────────────────────────────────────
-- 4. Events & Experiential Management
-- ──────────────────────────────────────────────────────────────────────
(
  'events-management',
  'Events & Experiential Management',
  '1.0.0',
  'For event management companies, experiential marketing agencies, MICE operators, wedding planners, and live entertainment producers. Supports the full event lifecycle from concept through post-event reconciliation, with multi-event portfolio management and client milestone billing.',
  '{
    "entity_labels": {
      "project": "Event",
      "project_plural": "Events",
      "phase": "Stage",
      "phase_plural": "Stages",
      "team_member": "Event Staff",
      "team_member_plural": "Event Staff",
      "rate_label": "Day Rate",
      "rate_unit": "day"
    },
    "phase_types": [
      {"id":"concept","label":"Concept & Proposal","order":1,"is_billable":false,"allowed_transitions":["planning","cancelled"]},
      {"id":"planning","label":"Planning & Design","order":2,"is_billable":true,"allowed_transitions":["production","concept","on_hold"]},
      {"id":"production","label":"Production & Procurement","order":3,"is_billable":true,"allowed_transitions":["rehearsal","pre_event","planning","on_hold"]},
      {"id":"rehearsal","label":"Rehearsal & Technical Checks","order":4,"is_billable":true,"allowed_transitions":["pre_event","production"]},
      {"id":"pre_event","label":"Pre-Event Setup (Bump-In)","order":5,"is_billable":true,"allowed_transitions":["live","rehearsal"]},
      {"id":"live","label":"Live / On-Site Event Day(s)","order":6,"is_billable":true,"allowed_transitions":["wrap"]},
      {"id":"wrap","label":"Wrap & Bump-Out","order":7,"is_billable":true,"allowed_transitions":["post_event"]},
      {"id":"post_event","label":"Post-Event & Reconciliation","order":8,"is_billable":false,"allowed_transitions":["completed"]},
      {"id":"on_hold","label":"On Hold","order":9,"is_billable":false,"allowed_transitions":["planning","production","cancelled"]},
      {"id":"completed","label":"Completed","order":10,"is_billable":false,"allowed_transitions":["archived"]},
      {"id":"cancelled","label":"Cancelled","order":11,"is_billable":false,"allowed_transitions":["archived"]},
      {"id":"archived","label":"Archived","order":12,"is_billable":false,"allowed_transitions":[]}
    ],
    "budget_categories": [
      {"id":"venue","label":"Venue & Space","description":"Venue hire, permissions, deposits, site surveys, security deposits","default_account_code":"5000"},
      {"id":"av_production","label":"AV & Technical Production","description":"Sound, lighting, LED walls, rigging, stage, technical crew","default_account_code":"5100"},
      {"id":"talent_entertainment","label":"Talent & Entertainment","description":"Performers, keynote speakers, MCs, bands, DJs, emcees","default_account_code":"5200"},
      {"id":"decor_theming","label":"Décor & Theming","description":"Floral, furniture, drapes, set design, props, signage, branding","default_account_code":"5300"},
      {"id":"catering","label":"Catering & Beverages","description":"Food, beverages, bar, staffing, crockery, linen","default_account_code":"5400"},
      {"id":"logistics_transport","label":"Logistics & Transport","description":"Freight, trucking, warehouse, delegate transport, parking","default_account_code":"5500"},
      {"id":"marketing_comms","label":"Marketing & Communications","description":"Invitations, branding, social media, photography, videography, PR","default_account_code":"5600"},
      {"id":"technology","label":"Event Technology","description":"Event app, registration platform, live streaming, virtual platform","default_account_code":"5700"},
      {"id":"staffing","label":"Event Staffing","description":"On-site event staff, ushers, registration desk, hosts, security","default_account_code":"5800"},
      {"id":"contingency","label":"Contingency","description":"Reserve for last-minute changes and overruns (typically 10%)","default_account_code":"8000"}
    ],
    "budget_templates": [
      {
        "name":"Corporate Conference (500 pax)",
        "description":"Full-day corporate conference with keynote, sessions, and dinner",
        "line_items":[
          {"category_id":"venue","description":"Conference Hall & Breakout Rooms","account_code":"5001","is_required":true},
          {"category_id":"av_production","description":"Full AV Setup (Sound, Lights, LED Screen)","account_code":"5101","is_required":true},
          {"category_id":"av_production","description":"Stage & Podium","account_code":"5102","is_required":true},
          {"category_id":"talent_entertainment","description":"Keynote Speaker","account_code":"5201","is_required":false},
          {"category_id":"talent_entertainment","description":"MC / Emcee","account_code":"5202","is_required":true},
          {"category_id":"decor_theming","description":"Stage Backdrop & Branding","account_code":"5301","is_required":true},
          {"category_id":"decor_theming","description":"Table Centrepieces & Floral","account_code":"5302","is_required":false},
          {"category_id":"catering","description":"Lunch & Coffee Breaks (per head)","account_code":"5401","is_required":true},
          {"category_id":"catering","description":"Gala Dinner","account_code":"5402","is_required":false},
          {"category_id":"logistics_transport","description":"Delegate Transport","account_code":"5501","is_required":false},
          {"category_id":"marketing_comms","description":"Invitations & RSVP Management","account_code":"5601","is_required":true},
          {"category_id":"marketing_comms","description":"Photography & Videography","account_code":"5602","is_required":true},
          {"category_id":"technology","description":"Registration Platform & Badge Printing","account_code":"5701","is_required":true},
          {"category_id":"staffing","description":"On-Site Event Staff (per day)","account_code":"5801","is_required":true},
          {"category_id":"contingency","description":"Contingency Reserve (10%)","account_code":"8001","is_required":true}
        ]
      },
      {
        "name":"Product Launch (Press & Influencer)",
        "description":"Brand product launch event for press, influencers, and trade",
        "line_items":[
          {"category_id":"venue","description":"Launch Venue Hire","account_code":"5001","is_required":true},
          {"category_id":"av_production","description":"AV & Lighting Production","account_code":"5101","is_required":true},
          {"category_id":"decor_theming","description":"Brand Installation & Set Design","account_code":"5301","is_required":true},
          {"category_id":"talent_entertainment","description":"Celebrity / Brand Ambassador","account_code":"5201","is_required":false},
          {"category_id":"catering","description":"Cocktail Reception Catering","account_code":"5403","is_required":true},
          {"category_id":"marketing_comms","description":"Influencer Gifting & Press Kits","account_code":"5603","is_required":true},
          {"category_id":"marketing_comms","description":"Live Social Media Coverage","account_code":"5604","is_required":true},
          {"category_id":"technology","description":"Live Streaming & Virtual Attendance","account_code":"5702","is_required":false},
          {"category_id":"contingency","description":"Contingency (10%)","account_code":"8001","is_required":true}
        ]
      },
      {
        "name":"Wedding (Full Service)",
        "description":"Full-service wedding coordination and management",
        "line_items":[
          {"category_id":"venue","description":"Wedding Venue (Ceremony + Reception)","account_code":"5001","is_required":true},
          {"category_id":"decor_theming","description":"Floral & Décor","account_code":"5302","is_required":true},
          {"category_id":"talent_entertainment","description":"Band / DJ / Performer","account_code":"5203","is_required":true},
          {"category_id":"catering","description":"Wedding Catering (per head)","account_code":"5401","is_required":true},
          {"category_id":"catering","description":"Wedding Cake","account_code":"5404","is_required":false},
          {"category_id":"av_production","description":"Sound & Lighting","account_code":"5103","is_required":true},
          {"category_id":"marketing_comms","description":"Photography & Videography","account_code":"5602","is_required":true},
          {"category_id":"logistics_transport","description":"Bridal Car & Guest Transport","account_code":"5502","is_required":false},
          {"category_id":"staffing","description":"Event Coordinators & Ushers","account_code":"5801","is_required":true},
          {"category_id":"contingency","description":"Contingency (10%)","account_code":"8001","is_required":true}
        ]
      }
    ],
    "expense_categories": [
      {"id":"venue_payment","label":"Venue Payment","tax_treatment":"input_gst","default_account_code":"5001","requires_po":true},
      {"id":"vendor_invoice","label":"Vendor / Supplier Invoice","tax_treatment":"input_gst","default_account_code":"5100","requires_po":true},
      {"id":"talent_fee","label":"Talent / Performer Fee","tax_treatment":"tds_applicable","default_account_code":"5201","requires_po":true},
      {"id":"catering_bill","label":"Catering Bill","tax_treatment":"input_gst","default_account_code":"5401","requires_po":true},
      {"id":"staff_payment","label":"Event Staff Payment","tax_treatment":"tds_applicable","default_account_code":"5801","requires_po":false},
      {"id":"petty_cash","label":"Petty Cash (On-Site)","tax_treatment":"none","default_account_code":"6099","requires_po":false},
      {"id":"reimbursement","label":"Staff Reimbursement","tax_treatment":"none","default_account_code":"6098","requires_po":false}
    ],
    "approval_workflow": {
      "limits":[
        {"role":"accountant","max_amount":"25000"},
        {"role":"coordinator","max_amount":"250000"},
        {"role":"manager","max_amount":"2000000"}
      ],
      "dual_approval_above":"2000000"
    },
    "inventory_categories": [
      {"id":"av_equipment","label":"AV & Technical Equipment","is_trackable":true,"depreciation_years":5},
      {"id":"furniture","label":"Furniture & Seating","is_trackable":true,"depreciation_years":7},
      {"id":"decor_props","label":"Décor, Props & Signage","is_trackable":false,"depreciation_years":null},
      {"id":"vehicle","label":"Event Vehicles","is_trackable":true,"depreciation_years":5},
      {"id":"technology_hardware","label":"Technology Hardware (Registration Kiosks, Tablets)","is_trackable":true,"depreciation_years":3},
      {"id":"linen_crockery","label":"Linen, Crockery & Tableware","is_trackable":false,"depreciation_years":null}
    ],
    "report_definitions": [
      {"id":"event_pl","name":"Event P&L","description":"Revenue vs. cost per event; management fee margin vs. pass-through costs","data_sources":["budget-planning","expense-tracking","general-ledger"],"default_columns":["event","client","revenue","pass_through_cost","direct_cost","gross_margin","margin_pct"]},
      {"id":"vendor_tracker","name":"Vendor & Supplier Tracker","description":"All POs raised, advances paid, invoices received, and balance due per vendor","data_sources":["expense-tracking"],"default_columns":["vendor","po_amount","advance_paid","invoice_received","balance_due","status"]},
      {"id":"client_invoice_schedule","name":"Client Invoice Schedule","description":"Milestone-based invoices issued to client, amounts, and payment status","data_sources":["general-ledger"],"default_columns":["event","milestone","invoice_no","amount","gst","total","issued_date","due_date","paid_date","status"]},
      {"id":"cost_per_head","name":"Cost Per Head Report","description":"Total event cost and cost breakdown per attending delegate / guest","data_sources":["budget-planning","expense-tracking"],"default_columns":["event","pax_count","total_cost","cost_per_head","category_breakdown"]},
      {"id":"post_event_reconciliation","name":"Post-Event Reconciliation","description":"Final budget vs. actual reconciliation issued to client after event wrap","data_sources":["budget-planning","expense-tracking"],"default_columns":["category","budgeted","actual","variance","notes"]}
    ],
    "custom_fields": {
      "project":[
        {"key":"client_name","label":"Client / Brand Name","type":"text","required":true},
        {"key":"event_type","label":"Event Type","type":"select","options":["Corporate Conference","Product Launch","Wedding","Gala Dinner","Exhibition","Concert","Sports Event","Virtual Event","Hybrid Event","Other"],"required":true},
        {"key":"expected_pax","label":"Expected Attendance (Pax)","type":"number","required":true},
        {"key":"event_date","label":"Event Date","type":"date","required":true},
        {"key":"venue_name","label":"Venue Name","type":"text","required":false},
        {"key":"client_po_number","label":"Client PO / Authorization Number","type":"text","required":false}
      ],
      "expense":[
        {"key":"vendor_confirmation_no","label":"Vendor Confirmation / Booking Reference","type":"text","required":false},
        {"key":"event_day","label":"Event Day (for multi-day events)","type":"text","required":false},
        {"key":"pass_through_to_client","label":"Pass-Through to Client?","type":"select","options":["Yes - at cost","Yes - with markup","No - absorbed"],"required":true}
      ]
    }
  }'::jsonb
)

ON CONFLICT (id) DO UPDATE
  SET name        = EXCLUDED.name,
      version     = EXCLUDED.version,
      description = EXCLUDED.description,
      config      = EXCLUDED.config,
      updated_at  = now();
