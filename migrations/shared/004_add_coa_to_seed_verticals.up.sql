-- 004_add_coa_to_seed_verticals.up.sql
-- Adds default_chart_of_accounts to the JSONB config for all 4 seeded verticals.
-- This field was omitted from migration 003 but is required for tenant registration
-- (step 4: seed chart of accounts in the general-ledger service).

-- Movie Production
UPDATE vertical_definitions
SET config = config || '{"default_chart_of_accounts":[
  {"code":"1000","name":"Cash and Bank","account_type":"asset","parent_code":null},
  {"code":"5100","name":"Above the Line Expenses","account_type":"expense","parent_code":null},
  {"code":"5200","name":"Below the Line Expenses","account_type":"expense","parent_code":null},
  {"code":"5300","name":"Post-Production Expenses","account_type":"expense","parent_code":null}
]}'::jsonb,
    updated_at = now()
WHERE id = 'movie-production';

-- Software Development
UPDATE vertical_definitions
SET config = config || '{"default_chart_of_accounts":[
  {"code":"1000","name":"Bank & Cash","account_type":"asset","parent_code":null},
  {"code":"1100","name":"Accounts Receivable","account_type":"asset","parent_code":null},
  {"code":"1200","name":"Unbilled Revenue (WIP)","account_type":"asset","parent_code":null},
  {"code":"1300","name":"Prepaid Expenses","account_type":"asset","parent_code":null},
  {"code":"2000","name":"Accounts Payable","account_type":"liability","parent_code":null},
  {"code":"2100","name":"TDS Payable","account_type":"liability","parent_code":null},
  {"code":"2200","name":"GST Payable (Output)","account_type":"liability","parent_code":null},
  {"code":"2300","name":"Salary Payable","account_type":"liability","parent_code":null},
  {"code":"3000","name":"Share Capital","account_type":"equity","parent_code":null},
  {"code":"3100","name":"Retained Earnings","account_type":"equity","parent_code":null},
  {"code":"4000","name":"Project Revenue","account_type":"revenue","parent_code":null},
  {"code":"4100","name":"Fixed-Price Revenue","account_type":"revenue","parent_code":"4000"},
  {"code":"4200","name":"T&M / Retainer Revenue","account_type":"revenue","parent_code":"4000"},
  {"code":"4300","name":"Support & AMC Revenue","account_type":"revenue","parent_code":"4000"},
  {"code":"5000","name":"Personnel Costs","account_type":"expense","parent_code":null},
  {"code":"5001","name":"Project Manager Cost","account_type":"expense","parent_code":"5000"},
  {"code":"5002","name":"UX / Design Cost","account_type":"expense","parent_code":"5000"},
  {"code":"5003","name":"Engineering Cost","account_type":"expense","parent_code":"5000"},
  {"code":"5004","name":"Frontend Engineering Cost","account_type":"expense","parent_code":"5000"},
  {"code":"5005","name":"QA Engineering Cost","account_type":"expense","parent_code":"5000"},
  {"code":"5006","name":"DevOps / SRE Cost","account_type":"expense","parent_code":"5000"},
  {"code":"6000","name":"Cloud Infrastructure","account_type":"expense","parent_code":null},
  {"code":"6001","name":"AWS / GCP / Azure Costs","account_type":"expense","parent_code":"6000"},
  {"code":"6100","name":"Software Licenses","account_type":"expense","parent_code":null},
  {"code":"6101","name":"SaaS Subscriptions","account_type":"expense","parent_code":"6100"},
  {"code":"6200","name":"Third-Party API & Services","account_type":"expense","parent_code":null},
  {"code":"6201","name":"API Usage Costs","account_type":"expense","parent_code":"6200"},
  {"code":"6300","name":"QA & Testing","account_type":"expense","parent_code":null},
  {"code":"6301","name":"Testing Tools & Licences","account_type":"expense","parent_code":"6300"},
  {"code":"6302","name":"Security Audit & Pen Test","account_type":"expense","parent_code":"6300"},
  {"code":"6500","name":"Travel & Onsite","account_type":"expense","parent_code":null},
  {"code":"6600","name":"Subcontractors","account_type":"expense","parent_code":null},
  {"code":"6700","name":"Hardware","account_type":"expense","parent_code":null},
  {"code":"6800","name":"Training & Certifications","account_type":"expense","parent_code":null},
  {"code":"8000","name":"Contingency","account_type":"expense","parent_code":null},
  {"code":"8001","name":"Contingency Reserve","account_type":"expense","parent_code":"8000"}
]}'::jsonb,
    updated_at = now()
WHERE id = 'software-development';

-- Construction
UPDATE vertical_definitions
SET config = config || '{"default_chart_of_accounts":[
  {"code":"1000","name":"Bank & Cash","account_type":"asset","parent_code":null},
  {"code":"1100","name":"Accounts Receivable","account_type":"asset","parent_code":null},
  {"code":"1200","name":"Retention Receivable","account_type":"asset","parent_code":null},
  {"code":"1300","name":"Materials on Site (Inventory)","account_type":"asset","parent_code":null},
  {"code":"1400","name":"Plant & Equipment (Net Book Value)","account_type":"asset","parent_code":null},
  {"code":"2000","name":"Accounts Payable","account_type":"liability","parent_code":null},
  {"code":"2100","name":"Retention Payable (to Subcontractors)","account_type":"liability","parent_code":null},
  {"code":"2200","name":"TDS Payable","account_type":"liability","parent_code":null},
  {"code":"2300","name":"GST Payable (Works Contract)","account_type":"liability","parent_code":null},
  {"code":"3000","name":"Share Capital","account_type":"equity","parent_code":null},
  {"code":"4000","name":"Contract Revenue","account_type":"revenue","parent_code":null},
  {"code":"4100","name":"Progress Billing Revenue","account_type":"revenue","parent_code":"4000"},
  {"code":"4200","name":"Variation Order Revenue","account_type":"revenue","parent_code":"4000"},
  {"code":"5000","name":"Labour Costs","account_type":"expense","parent_code":null},
  {"code":"5001","name":"Structural Labour","account_type":"expense","parent_code":"5000"},
  {"code":"5002","name":"Masonry Labour","account_type":"expense","parent_code":"5000"},
  {"code":"5003","name":"Finishing Labour","account_type":"expense","parent_code":"5000"},
  {"code":"6000","name":"Materials","account_type":"expense","parent_code":null},
  {"code":"6001","name":"Cement","account_type":"expense","parent_code":"6000"},
  {"code":"6002","name":"Steel / TMT Bars","account_type":"expense","parent_code":"6000"},
  {"code":"6003","name":"Bricks & Blocks","account_type":"expense","parent_code":"6000"},
  {"code":"6004","name":"Sand & Aggregates","account_type":"expense","parent_code":"6000"},
  {"code":"6005","name":"Tiles & Flooring","account_type":"expense","parent_code":"6000"},
  {"code":"6006","name":"Doors, Windows & Fixtures","account_type":"expense","parent_code":"6000"},
  {"code":"6007","name":"Bitumen & Asphalt","account_type":"expense","parent_code":"6000"},
  {"code":"6100","name":"Plant & Equipment","account_type":"expense","parent_code":null},
  {"code":"6101","name":"Plant Hire","account_type":"expense","parent_code":"6100"},
  {"code":"6200","name":"Subcontractors","account_type":"expense","parent_code":null},
  {"code":"6201","name":"Electrical Subcontractor","account_type":"expense","parent_code":"6200"},
  {"code":"6202","name":"Plumbing Subcontractor","account_type":"expense","parent_code":"6200"},
  {"code":"6203","name":"Painting & Waterproofing","account_type":"expense","parent_code":"6200"},
  {"code":"6300","name":"Site Overheads","account_type":"expense","parent_code":null},
  {"code":"6400","name":"Professional Fees","account_type":"expense","parent_code":null},
  {"code":"6500","name":"Testing & Inspection","account_type":"expense","parent_code":null},
  {"code":"8000","name":"Contingency","account_type":"expense","parent_code":null},
  {"code":"8001","name":"Contingency Reserve","account_type":"expense","parent_code":"8000"}
]}'::jsonb,
    updated_at = now()
WHERE id = 'construction';

-- Events Management
UPDATE vertical_definitions
SET config = config || '{"default_chart_of_accounts":[
  {"code":"1000","name":"Bank & Cash","account_type":"asset","parent_code":null},
  {"code":"1100","name":"Accounts Receivable","account_type":"asset","parent_code":null},
  {"code":"1200","name":"Vendor Advances & Deposits","account_type":"asset","parent_code":null},
  {"code":"2000","name":"Accounts Payable","account_type":"liability","parent_code":null},
  {"code":"2100","name":"Client Advances Received","account_type":"liability","parent_code":null},
  {"code":"2200","name":"TDS Payable","account_type":"liability","parent_code":null},
  {"code":"2300","name":"GST Payable","account_type":"liability","parent_code":null},
  {"code":"3000","name":"Share Capital","account_type":"equity","parent_code":null},
  {"code":"4000","name":"Event Management Revenue","account_type":"revenue","parent_code":null},
  {"code":"4100","name":"Management Fee Revenue","account_type":"revenue","parent_code":"4000"},
  {"code":"4200","name":"Pass-Through Vendor Revenue","account_type":"revenue","parent_code":"4000"},
  {"code":"5000","name":"Venue Costs","account_type":"expense","parent_code":null},
  {"code":"5100","name":"AV & Technical Production","account_type":"expense","parent_code":null},
  {"code":"5200","name":"Talent & Entertainment","account_type":"expense","parent_code":null},
  {"code":"5300","name":"Décor & Theming","account_type":"expense","parent_code":null},
  {"code":"5400","name":"Catering & Beverages","account_type":"expense","parent_code":null},
  {"code":"5500","name":"Logistics & Transport","account_type":"expense","parent_code":null},
  {"code":"5600","name":"Marketing & Communications","account_type":"expense","parent_code":null},
  {"code":"5700","name":"Event Technology","account_type":"expense","parent_code":null},
  {"code":"5800","name":"Event Staffing","account_type":"expense","parent_code":null},
  {"code":"8000","name":"Contingency","account_type":"expense","parent_code":null},
  {"code":"8001","name":"Contingency Reserve","account_type":"expense","parent_code":"8000"}
]}'::jsonb,
    updated_at = now()
WHERE id = 'events-management';
