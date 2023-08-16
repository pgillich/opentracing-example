-- https://ask.wireshark.org/question/15787/how-to-decode-protobuf-by-wireshark/
-- https://wiki.wireshark.org/Lua/Examples
-- https://wiki.wireshark.org/Lua/Dissectors
-- https://wiki.wireshark.org/Protobuf
-- https://ask.wireshark.org/question/25070/lua-how-to-get-a-field-from-a-decoded-protobuf-to-decode-the-next-protobuf/
-- https://ask.wireshark.org/question/31800/protobuf-dissector-with-nested-structures/
-- https://gist.github.com/xieran1988/2418151

local http_wrapper_proto = Proto("otlp_trace_http","OTLP Trace over HTTP")
-- (to confirm this worked, check that this protocol appears at the bottom of the "Filter Expression" dialog)
-- our new fields
local F_newfield1 = ProtoField.uint16("http.newfield1", "Our new field, #1", base.DEC)
local F_newfield2 = ProtoField.uint16("http.newfield2", "Our new field, #2", base.DEC)
-- add the fields to the protocol
-- (to confirm this worked, check that these fields appeared in the "Filter Expression" dialog)
http_wrapper_proto.fields = {F_newfield1, F_newfield2}          -- NOT ProtoFieldArray, that stopped working a while ago
local f_http_data = Field.new("http.file_data")
local original_http_dissector
function http_wrapper_proto.dissector(tvb, pinfo, treeitem)
    if tvb:len() == 0 then return end
    original_http_dissector:call(tvb, pinfo, treeitem)
    local body=f_http_data()
    if not body then return end
    local offset = 0
    print("tvb type: " .. type(tvb))
    print("tvb(offset, 4) type: " .. type(tvb(offset, 4)))
    print("body type: " .. type(body))
    print("body(offset, 4) type: " .. type(body(offset, 4)))
    local subtreeitem = treeitem:add(http_wrapper_proto, tvb)
    subtreeitem:add(F_newfield2, tvb:len())

        -- -- we've replaced the original http dissector in the dissector table,
        -- -- but we still want the original to run, especially because we need to read its data
        -- original_http_dissector:call(tvb, pinfo, treeitem)
        -- if f_set_cookie() then
        --         -- this has two effects:
        --         --      1. makes it so we can use "http_extra" as a display filter
        --         --      2. displays a new header in the tree pane for our protocol
        --         local subtreeitem = treeitem:add(http_wrapper_proto, tvb)
        --         field1_val = 42
        --         subtreeitem:add(F_newfield1, tvb(), field1_val)
        --                 :set_text("Don't panic: " .. field1_val)
        --         -- (now "http.newfield1 == 42" should work as a display filter)
        --         field2_val = 616
        --         subtreeitem:add(F_newfield2, tvb(), field2_val)
        --                 :set_text("The REAL number of the beast: " .. field2_val)
        -- end
end
local tcp_dissector_table = DissectorTable.get("tcp.port")
original_http_dissector = tcp_dissector_table:get_dissector(80) -- save the original dissector so we can still get to it
tcp_dissector_table:add(4318, http_wrapper_proto)                 -- and take its place in the dissector table